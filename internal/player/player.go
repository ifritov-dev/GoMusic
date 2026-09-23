// Package player streams audio files at the highest quality the source
// allows: native decode (mp3/flac/wav/ogg) -> float -> high-quality
// resample to a fixed 48kHz stereo output -> logarithmic volume gain.
// Playback is streaming (constant memory, tiny CPU) and cross-platform
// via bundled miniaudio: Linux (ALSA/Pulse), Windows (WASAPI),
// macOS (CoreAudio). No ffmpeg, no system dev packages, no external
// binaries.
package player

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"

	"github.com/ifritov-dev/GoMusic/internal/audio"
)

// OutRate is the fixed device rate. 48kHz covers all common sources
// (44.1k/48k/96k down- or up-sampled once, with beep's quality
// resampler) and is natively supported on every OS backend.
const OutRate = beep.SampleRate(48000)

type Player struct {
	mu sync.Mutex

	ctrl     *beep.Ctrl
	tap      *tapStreamer // visualizer tap, wraps ctrl
	vol      *effects.Volume
	streamer beep.StreamSeekCloser // resampled seekable stream (output-rate samples)
	file     *os.File
	win      []float32 // analysis scratch for Bands
	re, im   []float64 // FFT work buffers
	power    []float64 // power spectrum, bins 0..tapLen/2
	twRe     []float64 // FFT twiddles (built on first analyze)
	twIm     []float64

	done    chan struct{}
	audioOK bool
	paused  bool
	volume  int // 0-100
	muted   bool
	curPath string
	lenOut  int // length in output-rate samples
	// Source info for display (format badge).
	srcExt  string
	srcRate int
	srcCh   int
	srcPrec int
}

// New creates a player. Init must be called before use.
func New(volume int) *Player {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	return &Player{volume: volume, done: make(chan struct{}),
		win:   make([]float32, tapLen),
		re:    make([]float64, tapLen),
		im:    make([]float64, tapLen),
		power: make([]float64, tapLen/2+1),
	}
}

// Init opens the OS audio device. If it fails (e.g. headless CI),
// the player keeps working in silent mode so the TUI stays usable.
func (p *Player) Init() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctrl = &beep.Ctrl{}
	p.tap = newTapStreamer(p.ctrl)
	p.vol = &effects.Volume{Streamer: p.tap, Base: 2, Volume: gain(p.volume), Silent: p.muted}
	if err := audio.Init(int(OutRate)); err != nil {
		p.audioOK = false
		return
	}
	p.audioOK = true
	audio.Play(p.vol)
}

func gain(pct int) float64 {
	if pct <= 0 {
		// math.Pow(2, -Inf) == 0, so the mixer outputs true
		// silence (the old -5 floor leaked ~3% amplitude).
		return math.Inf(-1)
	}
	if pct >= 100 {
		return 0
	}
	return math.Log2(float64(pct) / 100.0)
}

func (p *Player) applyVolumeLocked() {
	if p.vol == nil {
		return
	}
	p.vol.Volume = gain(p.volume)
	p.vol.Silent = p.muted
}

// decode opens path and returns a seek-closer + format.
func decode(path string, f *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return mp3.Decode(f)
	case ".wav":
		return wav.Decode(f)
	case ".flac":
		return flac.Decode(f)
	case ".ogg", ".oga":
		return vorbis.Decode(f)
	default:
		return mp3.Decode(f)
	}
}

// PlayFile stops the current track and starts streaming path.
func (p *Player) PlayFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	s, format, err := decode(path, f)
	if err != nil {
		f.Close()
		return err
	}

	// Resample to output rate only when needed — sources already at
	// 48k pass through untouched (bit-perfect apart from float conv).
	var seeker beep.StreamSeekCloser = s
	length := s.Len()
	if format.SampleRate != OutRate {
		ratio := float64(format.SampleRate) / float64(OutRate)
		length = int(float64(s.Len()) / ratio)
		seeker = newResampleSeeker(s, ratio, length)
	}

	p.mu.Lock()
	if p.ctrl != nil {
		audio.Lock()
		p.ctrl.Streamer = nil
		audio.Unlock()
	}
	if p.streamer != nil {
		p.streamer.Close()
	}
	if p.file != nil {
		p.file.Close()
	}
	// Release old waiters.
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	p.done = make(chan struct{})
	done := p.done

	p.file = f
	p.streamer = seeker
	p.curPath = path
	p.lenOut = length
	p.srcExt = strings.ToUpper(strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."))
	p.srcRate = int(format.SampleRate)
	p.srcCh = format.NumChannels
	p.srcPrec = format.Precision
	p.paused = false

	finished := beep.Seq(seeker, beep.Callback(func() {
		// Runs on the audio thread: never block.
		select {
		case <-done:
		default:
			close(done)
		}
	}))
	if p.ctrl == nil {
		p.ctrl = &beep.Ctrl{Streamer: finished}
		p.tap = newTapStreamer(p.ctrl)
		p.vol = &effects.Volume{Streamer: p.tap, Base: 2, Volume: gain(p.volume), Silent: p.muted}
		if p.audioOK {
			audio.Play(p.vol)
		}
	} else {
		audio.Lock()
		p.ctrl.Paused = false
		p.ctrl.Streamer = finished
		p.applyVolumeLocked()
		audio.Unlock()
	}
	p.mu.Unlock()
	return nil
}

// Done returns a channel closed when the current track finishes.
func (p *Player) Done() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

func (p *Player) Current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.curPath
}

// Info describes the current source for display, e.g. ("FLAC", 44100, 2).
// Empty ext means nothing is loaded.
func (p *Player) Info() (ext string, rateHz int, channels int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srcExt, p.srcRate, p.srcCh
}

func (p *Player) Paused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

func (p *Player) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = !p.paused
	if p.ctrl != nil {
		audio.Lock()
		p.ctrl.Paused = p.paused
		audio.Unlock()
	}
}

func (p *Player) SetPaused(b bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = b
	if p.ctrl != nil {
		audio.Lock()
		p.ctrl.Paused = b
		audio.Unlock()
	}
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctrl != nil {
		audio.Lock()
		p.ctrl.Streamer = nil
		audio.Unlock()
	}
	if p.streamer != nil {
		p.streamer.Close()
		p.streamer = nil
	}
	if p.file != nil {
		p.file.Close()
		p.file = nil
	}
	p.curPath = ""
	p.lenOut = 0
	p.srcExt = ""
	p.srcRate = 0
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	p.done = make(chan struct{})
}

// Volume / mute ---------------------------------------------------

func (p *Player) Volume() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

func (p *Player) SetVolume(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = v
	if v > 0 && p.muted {
		// Raising volume unmutes (standard UX).
		p.muted = false
	}
	p.applyVolumeLocked()
}

func (p *Player) Muted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.muted
}

func (p *Player) ToggleMute() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.muted = !p.muted
	p.applyVolumeLocked()
	return p.muted
}

// Position / duration (output-rate samples -> time) ---------------

func (p *Player) Position() time.Duration {
	p.mu.Lock()
	s := p.streamer
	p.mu.Unlock()
	if s == nil {
		return 0
	}
	audio.Lock()
	pos := s.Position()
	audio.Unlock()
	return time.Duration(float64(pos) / float64(OutRate) * float64(time.Second))
}

func (p *Player) Length() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lenOut <= 0 {
		return 0
	}
	return time.Duration(float64(p.lenOut) / float64(OutRate) * float64(time.Second))
}

// SeekBy moves playback by d (clamped). Returns new position.
func (p *Player) SeekBy(d time.Duration) time.Duration {
	p.mu.Lock()
	s := p.streamer
	p.mu.Unlock()
	if s == nil {
		return 0
	}
	samples := int(float64(d) / float64(time.Second) * float64(OutRate))
	audio.Lock()
	pos := s.Position() + samples
	audio.Unlock()
	return p.seekToSamples(pos)
}

// SeekTo jumps to absolute position d (clamped). Returns new position.
func (p *Player) SeekTo(d time.Duration) time.Duration {
	return p.seekToSamples(int(float64(d) / float64(time.Second) * float64(OutRate)))
}

func (p *Player) seekToSamples(pos int) time.Duration {
	p.mu.Lock()
	s := p.streamer
	n := p.lenOut
	p.mu.Unlock()
	if s == nil {
		return 0
	}
	if pos < 0 {
		pos = 0
	}
	if n > 0 && pos >= n {
		pos = n - 1
	}
	audio.Lock()
	_ = s.Seek(pos)
	audio.Unlock()
	return time.Duration(float64(pos) / float64(OutRate) * float64(time.Second))
}

func (p *Player) Close() {
	p.Stop()
	audio.Close()
}

// resampleSeeker adapts beep's non-seekable Resampler to a
// StreamSeekCloser in output-rate samples. Seeking re-seeks the
// source and rebuilds the resampler at that offset (the
// interpolation window restarts — inaudible, sample-accurate target).
type resampleSeeker struct {
	src     beep.StreamSeekCloser
	rs      *beep.Resampler
	ratio   float64 // srcRate / outRate
	quality int
	pos     int
	length  int
}

func newResampleSeeker(src beep.StreamSeekCloser, ratio float64, length int) *resampleSeeker {
	return &resampleSeeker{
		src:     src,
		rs:      beep.ResampleRatio(4, ratio, src),
		ratio:   ratio,
		quality: 4,
		length:  length,
	}
}

func (r *resampleSeeker) Stream(samples [][2]float64) (int, bool) {
	n, ok := r.rs.Stream(samples)
	r.pos += n
	return n, ok
}

func (r *resampleSeeker) Err() error { return r.src.Err() }

func (r *resampleSeeker) Len() int { return r.length }

func (r *resampleSeeker) Position() int { return r.pos }

func (r *resampleSeeker) Seek(p int) error {
	if p < 0 {
		p = 0
	}
	if p > r.length {
		p = r.length
	}
	if err := r.src.Seek(int(float64(p) * r.ratio)); err != nil {
		return err
	}
	r.rs = beep.ResampleRatio(r.quality, r.ratio, r.src)
	r.pos = p
	return nil
}

func (r *resampleSeeker) Close() error { return r.src.Close() }
