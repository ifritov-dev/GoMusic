package player

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/gopxl/beep/v2"
)

// sineStreamer feeds a continuous sine so the tap window always ends
// up full of a known tone.
type sineStreamer struct {
	freq, amp, rate float64
	phase           float64
}

func (s *sineStreamer) Stream(out [][2]float64) (int, bool) {
	for i := range out {
		v := s.amp * math.Sin(2*math.Pi*s.phase)
		out[i] = [2]float64{v, v}
		if s.phase += s.freq / s.rate; s.phase >= 1 {
			s.phase--
		}
	}
	return len(out), true
}

func (s *sineStreamer) Err() error { return nil }

// TestTapWindowOrder pins the ring buffer read order: bars look up the
// newest samples first, a reversed window would silently draw noise.
func TestTapWindowOrder(t *testing.T) {
	v := 0.0
	src := beep.StreamerFunc(func(out [][2]float64) (int, bool) {
		for i := range out {
			out[i] = [2]float64{v, v}
			v++
		}
		return len(out), true
	})
	tap := newTapStreamer(src)
	buf := make([][2]float64, tapLen)

	if n, ok := tap.Stream(buf); n != tapLen || !ok {
		t.Fatalf("tap dropped samples: n=%d ok=%v", n, ok)
	}
	win := make([]float32, tapLen)
	tap.recent(win)
	if win[0] != 0 || win[tapLen-1] != tapLen-1 {
		t.Fatalf("full window not chronological: first=%v last=%v", win[0], win[tapLen-1])
	}

	// Wrap around: the window must slide to the newest tapLen samples.
	if _, ok := tap.Stream(buf[:512]); !ok {
		t.Fatal("tap stopped early")
	}
	tap.recent(win)
	if win[0] != 512 || win[tapLen-1] != tapLen-1+512 {
		t.Fatalf("wrapped window wrong: first=%v last=%v", win[0], win[tapLen-1])
	}
	// The passthrough is untouched (the ring only reads).
	if buf[0][0] != tapLen {
		t.Fatalf("tap altered the samples it forwarded: %v", buf[0][0])
	}
}

// TestBandsPeakAtTone checks the analyzer points at the tone's band
// and stays quiet everywhere else.
func TestBandsPeakAtTone(t *testing.T) {
	src := &sineStreamer{freq: 1000, amp: 0.9, rate: float64(OutRate)}
	tap := newTapStreamer(src)
	buf := make([][2]float64, 512)
	for i := 0; i < 4; i++ {
		tap.Stream(buf) // 2048 samples = one full window
	}
	p := New(100)
	p.tap = tap

	const n = 48
	bands := p.Bands(make([]float64, n))
	if len(bands) != n {
		t.Fatalf("Bands returned %d levels, want %d", len(bands), n)
	}
	peer := 0
	for i, v := range bands {
		if v > bands[peer] {
			peer = i
		}
	}
	// The peaking band must be the one whose range covers 1 kHz.
	if !(bandHz(peer, n) <= 1000 && 1000 < bandHz(peer+1, n)) {
		t.Fatalf("peak band %d is %.0f..%.0f Hz, want the 1 kHz band", peer, bandHz(peer, n), bandHz(peer+1, n))
	}
	if bands[peer] < 0.8 {
		t.Fatalf("1 kHz tone too quiet: %.2f", bands[peer])
	}
	if bands[0] > 0.2 || bands[n-1] > 0.3 {
		t.Fatalf("tone leaked: 50 Hz=%.2f 10 kHz=%.2f", bands[0], bands[n-1])
	}
}

// TestBandsReachesTopBand is the regression guard for the top of the
// range: with a 15 kHz ceiling the last bars were permanently dark
// (no music energy up there), which read as "the visualizer is
// missing its last bar".
func TestBandsReachesTopBand(t *testing.T) {
	src := &sineStreamer{freq: 9500, amp: 0.9, rate: float64(OutRate)}
	tap := newTapStreamer(src)
	buf := make([][2]float64, 512)
	for i := 0; i < 4; i++ {
		tap.Stream(buf)
	}
	p := New(100)
	p.tap = tap
	const n = 48
	bands := p.Bands(make([]float64, n))
	if bands[n-1] < 0.5 {
		t.Fatalf("top band (%.0f Hz) reads %.2f for a 9.5 kHz tone", bandHz(n-1, n), bands[n-1])
	}
}

// TestBandsSilentWithoutTap covers the headless/CI case: no audio
// device means no tap, and Bands must fill zeros instead of panicking.
func TestBandsSilentWithoutTap(t *testing.T) {
	p := New(100)
	for _, n := range []int{1, 16, 64} {
		for i, v := range p.Bands(make([]float64, n)) {
			if v != 0 {
				t.Fatalf("n=%d band %d = %v, want 0", n, i, v)
			}
		}
	}
	if got := p.Bands(nil); len(got) != 0 {
		t.Fatalf("empty dst grew to %d", len(got))
	}
}

// bandHz is the lower edge of band i of n (the band spans it to
// bandHz(i+1)); bandHz(n, n) is the top of the range.
func bandHz(i, n int) float64 {
	return bandLo * math.Pow(bandHi/bandLo, float64(i)/float64(n))
}

// TestChainReachesTap proves the tap is really in the playback chain:
// the volume stage must pull through it, and a playing file must land
// in the spectrum. No audio device is needed — the pull is manual
// (with a device present the realtime callback owns the stream, so
// the test steps aside).
func TestChainReachesTap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	writeWav(t, path, 48000, 0.5) // 440 Hz sine, amplitude 0.5
	p := New(100)
	p.Init()
	defer p.Close()
	if p.audioOK {
		t.Skip("audio device present: the realtime callback owns the stream")
	}
	if err := p.PlayFile(path); err != nil {
		t.Fatal(err)
	}
	if p.vol.Streamer != beep.Streamer(p.tap) {
		t.Fatal("the volume stage does not pull through the visualizer tap")
	}
	buf := make([][2]float64, 512)
	for i := 0; i < 8; i++ { // 4096 samples: two full windows
		if n, _ := p.vol.Stream(buf); n != len(buf) {
			t.Fatalf("chain starved after %d frames", i)
		}
	}
	const n = 48
	bands := p.Bands(make([]float64, n))
	peak := 0
	for i, v := range bands {
		if v > bands[peak] {
			peak = i
		}
	}
	if peak < 17 || peak > 22 { // 440 Hz sits in band 19
		t.Fatalf("peak band %d (%.0f Hz), want the 440 Hz band", peak, bandHz(peak, n))
	}
	if bands[peak] < 0.4 {
		t.Fatalf("playing track reads quiet: %.2f", bands[peak])
	}
}
