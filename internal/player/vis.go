// Visualizer tap: a streamer inserted between the transport (Ctrl)
// and the volume gain. It forwards samples untouched and keeps a
// rolling window of mono samples that the UI turns into spectrum
// bars. Tapping pre-volume means muted music still animates; tapping
// post-transport means a pause stops feeding the window, so the bars
// fall silent instead of freezing.
package player

import (
	"math"
	"sync"

	"github.com/gopxl/beep/v2"
)

const (
	tapLen      = 2048    // analysis window: 43 ms at 48 kHz
	bandLo      = 50.0    // lowest band (Hz), CAVA's lower_cutoff_freq
	bandHi      = 10000.0 // highest band (Hz), CAVA's higher_cutoff_freq
	bandFloorDb = 70.0    // levels below -70 dBFS read as 0
)

// levelGamma lifts the middle of the range, so normal listening levels
// fill the bars instead of hovering near the floor (CAVA's sensitivity).
const levelGamma = 0.8

// hann is the analysis window, precomputed once: the FFT needs it to
// keep transients from smearing across the whole spectrum.
var hann = func() []float64 {
	w := make([]float64, tapLen)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(tapLen-1))
	}
	return w
}()

// tapStreamer passes samples through and mirrors them (mono) into a
// ring buffer. It owns its mutex and never touches player state:
// Stream runs on the audio thread under audio.Lock.
type tapStreamer struct {
	src beep.Streamer
	mu  sync.Mutex
	buf []float32
	w   int // next write slot == oldest sample
}

func newTapStreamer(src beep.Streamer) *tapStreamer {
	return &tapStreamer{src: src, buf: make([]float32, tapLen)}
}

func (t *tapStreamer) Stream(samples [][2]float64) (int, bool) {
	n, ok := t.src.Stream(samples)
	t.mu.Lock()
	for i := 0; i < n; i++ {
		t.buf[t.w] = float32((samples[i][0] + samples[i][1]) * 0.5)
		if t.w++; t.w == len(t.buf) {
			t.w = 0
		}
	}
	t.mu.Unlock()
	return n, ok
}

func (t *tapStreamer) Err() error { return t.src.Err() }

// recent copies the newest len(dst) samples in chronological order.
// len(dst) must be <= tapLen.
func (t *tapStreamer) recent(dst []float32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	start := t.w - len(dst)
	if start < 0 {
		start += len(t.buf)
	}
	i := copy(dst, t.buf[start:])
	copy(dst[i:], t.buf[:len(dst)-i])
}

// Bands fills dst with len(dst) logarithmically spaced band levels in
// 0..1, from bandLo to bandHi. Each level is the band's RMS amplitude
// in dB (a single tone reads the same wherever it sits, whole bands
// of energy read loud). Safe with nothing loaded (fills zeros); meant
// for one caller at animation rate — it reuses internal scratch
// buffers.
func (p *Player) Bands(dst []float64) []float64 {
	if len(dst) == 0 {
		return dst
	}
	p.mu.Lock()
	t := p.tap
	p.mu.Unlock()
	if t == nil {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}
	p.analyze(t)

	binHz := float64(OutRate) / float64(tapLen)
	// Hann coherent gain 0.5: a full-scale sine puts N/4 into its bin.
	ref := float64(tapLen) / 4
	// Bands tile the range edge to edge (geometric steps), so the top
	// band is a real band: defining it as [bottom, bandHi] of its own
	// centre would leave the last bar sampling a single bin forever.
	r := math.Pow(bandHi/bandLo, 1/float64(len(dst)))
	for i := range dst {
		fLo := bandLo * math.Pow(r, float64(i))
		fHi := fLo * r
		k0 := max(int(math.Ceil(fLo/binHz)), 1) // bin 0 is DC
		k1 := min(int(math.Floor(fHi/binHz)), tapLen/2)
		if k1 < k0 {
			// Band narrower than one bin (bottom of the range):
			// read the nearest bin instead of nothing.
			k1 = clampBin(int(math.Round((fLo + fHi) / 2 / binHz)))
			k0 = k1
		}
		var sum float64
		for k := k0; k <= k1; k++ {
			sum += p.power[k]
		}
		amp := math.Sqrt(sum/float64(k1-k0+1)) / ref
		// Gentle treble tilt: music rolls off up here, a flat
		// spectrum would leave the right half of the bars dead.
		amp *= math.Pow((fLo+fHi)/2/200, 0.3)
		db := 20 * math.Log10(math.Max(amp, 1e-9))
		lvl := math.Min(math.Max((db+bandFloorDb)/bandFloorDb, 0), 1)
		dst[i] = math.Pow(lvl, levelGamma)
	}
	return dst
}

func clampBin(k int) int { return min(max(k, 1), tapLen/2) }

// analyze computes the power spectrum (bins 0..tapLen/2) of the newest
// window. Iterative radix-2 Cooley-Tukey; the twiddles are built once.
func (p *Player) analyze(t *tapStreamer) {
	n := tapLen
	if p.twRe == nil {
		p.twRe, p.twIm = make([]float64, n/2), make([]float64, n/2)
		for k := range p.twRe {
			ang := -2 * math.Pi * float64(k) / float64(n)
			p.twRe[k], p.twIm[k] = math.Cos(ang), math.Sin(ang)
		}
	}
	t.recent(p.win)
	re, im := p.re, p.im
	for i := 0; i < n; i++ {
		re[i], im[i] = float64(p.win[i])*hann[i], 0
	}
	for i, j := 1, 0; i < n; i++ { // bit-reversal permutation
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		half, step := length/2, n/length
		for i := 0; i < n; i += length {
			for k := 0; k < half; k++ {
				cr, ci := p.twRe[k*step], p.twIm[k*step]
				a, b := i+k, i+k+half
				tr := re[b]*cr - im[b]*ci
				ti := re[b]*ci + im[b]*cr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
			}
		}
	}
	for k := 0; k <= n/2; k++ {
		p.power[k] = re[k]*re[k] + im[k]*im[k]
	}
}
