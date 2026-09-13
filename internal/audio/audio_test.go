package audio

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/gopxl/beep/v2"
)

// TestNullBackendStreams verifies the realtime callback actually pulls
// samples through the full chain (miniaudio null device = no hardware).
func TestNullBackendStreams(t *testing.T) {
	if err := InitWithBackends(48000, []malgo.Backend{malgo.BackendNull}); err != nil {
		t.Skipf("null backend unavailable: %v", err)
	}
	defer Close()

	var frames atomic.Int64
	s := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		for i := range samples {
			samples[i] = [2]float64{0.1, 0.1}
		}
		frames.Add(int64(len(samples)))
		return len(samples), true
	})
	Play(s)
	time.Sleep(300 * time.Millisecond)
	if got := frames.Load(); got < 4800 { // ≥0.1s of audio at 48k
		t.Fatalf("only %d frames streamed in 300ms", got)
	}

	// Drained streamer -> silence, no crash, no spin.
	done := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		return 0, false
	})
	Set(done)
	time.Sleep(100 * time.Millisecond)
}
