package player

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"
)

// writeWav writes a stereo PCM16 sine WAV for decoder tests.
func writeWav(t *testing.T, path string, sampleRate int, secs float64) {
	t.Helper()
	n := int(float64(sampleRate) * secs)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dataSize := n * 2 * 2 // stereo int16
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+dataSize))
	copy(hdr[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:], 2) // stereo
	binary.LittleEndian.PutUint32(hdr[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:], uint32(sampleRate*4))
	binary.LittleEndian.PutUint16(hdr[32:], 4)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(dataSize))
	if _, err := f.Write(hdr); err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, dataSize)
	for i := 0; i < n; i++ {
		v := int16(0.5 * 32767 * math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
	}
	if _, err := f.Write(pcm); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeWavNativeRate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.wav")
	writeWav(t, p, 48000, 1.0)
	f, _ := os.Open(p)
	defer f.Close()
	s, format, err := decode(p, f)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if format.SampleRate != beep.SampleRate(48000) {
		t.Fatalf("rate = %d", format.SampleRate)
	}
	if s.Len() != 48000 {
		t.Fatalf("len = %d, want 48000", s.Len())
	}
}

func TestResampleSeekerLengthAndSeek(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b.wav")
	writeWav(t, p, 44100, 2.0) // 88200 source samples
	f, _ := os.Open(p)
	defer f.Close()
	raw, _, err := wav.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	ratio := 44100.0 / 48000.0
	wantLen := int(float64(raw.Len()) / ratio)
	rs := newResampleSeeker(raw, ratio, wantLen)
	if rs.Len() != wantLen {
		t.Fatalf("len = %d want %d", rs.Len(), wantLen)
	}
	// Stream a bit, check position advances.
	buf := make([][2]float64, 512)
	n, _ := rs.Stream(buf)
	if n == 0 || rs.Position() != n {
		t.Fatalf("streamed %d pos %d", n, rs.Position())
	}
	// Seek to 1s in output domain.
	if err := rs.Seek(48000); err != nil {
		t.Fatal(err)
	}
	if rs.Position() != 48000 {
		t.Fatalf("pos = %d want 48000", rs.Position())
	}
	// Seek clamps.
	if err := rs.Seek(-5); err != nil {
		t.Fatal(err)
	}
	if rs.Position() != 0 {
		t.Fatalf("pos = %d want 0", rs.Position())
	}
	if err := rs.Seek(wantLen + 1000); err != nil {
		t.Fatal(err)
	}
	if rs.Position() != wantLen {
		t.Fatalf("pos = %d want %d", rs.Position(), wantLen)
	}
}

func TestGainMapping(t *testing.T) {
	if gain(100) != 0 {
		t.Fatalf("gain(100) = %v", gain(100))
	}
	if math.Abs(gain(50)-(-1)) > 1e-9 {
		t.Fatalf("gain(50) = %v want -1", gain(50))
	}
	if gain(0) >= gain(50) {
		t.Fatal("gain(0) should be quieter than gain(50)")
	}
	if !math.IsInf(gain(0), -1) {
		t.Fatalf("gain(0) = %v, want -Inf for true silence", gain(0))
	}
	if got := math.Pow(2, gain(0)); got != 0 {
		t.Fatalf("0%% amplitude multiplier = %v, want exactly 0", got)
	}
}

func TestPlayerSilentModeQueue(t *testing.T) {
	// No audio device needed: Init must not crash, controls must work.
	p := New(80)
	p.Init()
	defer p.Close()
	dir := t.TempDir()
	p1 := filepath.Join(dir, "one.wav")
	p2 := filepath.Join(dir, "two.wav")
	writeWav(t, p1, 48000, 0.5)
	writeWav(t, p2, 48000, 0.5)
	if err := p.PlayFile(p1); err != nil {
		t.Fatal(err)
	}
	if p.Current() != p1 {
		t.Fatal("current mismatch")
	}
	if p.Length() <= 0 {
		t.Fatal("length should be > 0")
	}
	p.Toggle()
	if !p.Paused() {
		t.Fatal("should be paused")
	}
	p.Toggle()
	if p.Paused() {
		t.Fatal("should be playing")
	}
	p.SetVolume(33)
	if p.Volume() != 33 {
		t.Fatal("volume mismatch")
	}
	if !p.ToggleMute() || !p.Muted() {
		t.Fatal("should be muted")
	}
	p.SetVolume(50) // raising volume unmutes
	if p.Muted() {
		t.Fatal("should be unmuted")
	}
	if err := p.PlayFile(p2); err != nil {
		t.Fatal(err)
	}
	p.Stop()
	if p.Current() != "" || p.Length() != 0 {
		t.Fatal("stop should clear state")
	}
}

func TestSeekToAbsolute(t *testing.T) {
	p := New(80)
	p.Init()
	defer p.Close()
	dir := t.TempDir()
	f := filepath.Join(dir, "s.wav")
	writeWav(t, f, 48000, 2.0)
	if err := p.PlayFile(f); err != nil {
		t.Fatal(err)
	}
	if got := p.SeekTo(1500 * time.Millisecond); got < 1400*time.Millisecond || got > 1600*time.Millisecond {
		t.Fatalf("seek to 1.5s = %v", got)
	}
	if pos := p.Position(); pos < 1400*time.Millisecond || pos > 1600*time.Millisecond {
		t.Fatalf("position after seek = %v", pos)
	}
	// Negative clamps to 0, past-the-end clamps to length.
	if got := p.SeekTo(-5 * time.Second); got != 0 {
		t.Fatalf("negative seek = %v, want 0", got)
	}
	if length := p.Length(); length <= 0 {
		t.Fatal("length should be > 0")
	} else if got := p.SeekTo(length + 5*time.Second); got < 0 || got >= length {
		t.Fatalf("past-end seek = %v, length %v", got, length)
	}
	// SeekBy still works (relative on top of absolute).
	p.SeekTo(0)
	if got := p.SeekBy(500 * time.Millisecond); got < 400*time.Millisecond || got > 600*time.Millisecond {
		t.Fatalf("seek +0.5s = %v", got)
	}
}
