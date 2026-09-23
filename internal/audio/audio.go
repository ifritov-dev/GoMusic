// Package audio is a minimal, dependency-free speaker backend.
//
// It pulls float samples from a single beep.Streamer (typically an
// effects.Volume wrapping a beep.Ctrl, exactly like the classic
// beep/speaker design) and feeds them to the OS audio device via
// miniaudio (gen2brain/malgo), which is bundled as C code — no ALSA
// headers, no pkg-config, no external binaries on any platform.
//
// Output is fixed at 48kHz / stereo / float32. Sources are resampled
// to this rate once by the player (beep.Resample, quality=4), so this
// backend never touches sample quality — it only converts float64 to
// float32 for the device.
package audio

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/gen2brain/malgo"
	"github.com/gopxl/beep/v2"
)

var (
	mu      sync.Mutex
	current beep.Streamer
	buf     [][2]float64

	ctx     *malgo.AllocatedContext
	dev     *malgo.Device
	started bool
)

// Init opens the default output device at sampleRate Hz.
// Failure (e.g. headless machine) is reported so the caller can run
// in silent mode — the TUI stays fully usable.
func Init(sampleRate int) error {
	return InitWithBackends(sampleRate, nil)
}

// InitWithBackends is Init with an explicit miniaudio backend list
// (nil = all). Tests use []malgo.Backend{malgo.BackendNull} to drive
// the realtime callback without hardware.
func InitWithBackends(sampleRate int, backends []malgo.Backend) error {
	// NOTE: never hold mu across InitDevice/Start — Start() invokes
	// the data callback synchronously, and it needs mu (deadlock).
	mu.Lock()
	if started {
		mu.Unlock()
		return nil
	}
	mu.Unlock()

	c, err := malgo.InitContext(backends, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = 2
	cfg.SampleRate = uint32(sampleRate)
	cfg.Alsa.NoMMap = 1

	callbacks := malgo.DeviceCallbacks{
		Data: func(pOut, _ []byte, frameCount uint32) {
			fill(pOut, int(frameCount))
		},
	}
	d, err := malgo.InitDevice(c.Context, cfg, callbacks)
	if err != nil {
		c.Free()
		return err
	}
	if err := d.Start(); err != nil {
		d.Uninit()
		c.Free()
		return err
	}
	mu.Lock()
	ctx, dev, started = c, d, true
	mu.Unlock()
	return nil
}

// fill renders frameCount stereo f32 frames into pOut.
// Called on miniaudio's realtime thread — keep it lock-short.
func fill(pOut []byte, frames int) {
	mu.Lock()
	s := current
	need := frames
	if cap(buf) < need {
		buf = make([][2]float64, need)
	}
	buf = buf[:need]
	var n int
	var ok bool
	if s != nil {
		n, ok = s.Stream(buf)
		_ = ok
	}
	mu.Unlock()

	// Clamp + convert to little-endian float32.
	off := 0
	for i := 0; i < frames; i++ {
		var l, r float64
		if i < n {
			l, r = buf[i][0], buf[i][1]
		}
		if off+8 > len(pOut) {
			break
		}
		binary.LittleEndian.PutUint32(pOut[off:], math.Float32bits(float32(l)))
		binary.LittleEndian.PutUint32(pOut[off+4:], math.Float32bits(float32(r)))
		off += 8
	}
	for ; off < len(pOut); off++ {
		pOut[off] = 0
	}
}

// Play sets the source streamer (call once, then use Set to swap).
func Play(s beep.Streamer) { Set(s) }

// Set swaps the source streamer.
func Set(s beep.Streamer) {
	mu.Lock()
	defer mu.Unlock()
	current = s
}

// Lock/Unlock guard streamer mutations (pause, seek, swap),
// mirroring the beep/speaker API.
func Lock()   { mu.Lock() }
func Unlock() { mu.Unlock() }

// Close shuts down the device.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if dev != nil {
		dev.Stop()
		dev.Uninit()
		dev = nil
	}
	if ctx != nil {
		ctx.Free()
		ctx = nil
	}
	started = false
	current = nil
}
