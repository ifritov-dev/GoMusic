// Spectrum visualizer ("v"): full-width bars over the cover-art
// gradient, animated at 60 fps while it is on. Levels come from the
// player's stream tap, so nothing plays double; a pause settles the
// bars on a low resting row instead of clearing the screen.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// visTickMsg drives the bar animation (separate from the 500 ms
// housekeeping tick, which is far too slow for motion).
type visTickMsg struct{}

const (
	visFrame = 16 * time.Millisecond // 60 fps: CAVA's default framerate
	// Integral filter: a bar eases toward a louder frame instead of
	// snapping, which is what removes the frame-to-frame jitter
	// (a beat still lands in ~4 frames).
	visIntegral = 0.35
	// Gravity filter: falling bars lose visFall of their height per
	// frame plus a constant pull, so they drop in ~0.5s and land
	// instead of creeping forever.
	visFall = 0.90
	visPull = 0.012
	visBarW = 2 // cells per bar, 1 cell gap (CAVA's bar_width/spacing)
	// visFloor is where the bars rest while playback is paused: a low
	// row of stubs, so the screen reads "loaded, waiting" instead of
	// empty.
	visFloor = 0.06
)

// barGlyphs are the partial block heights (1/8 cell steps) that make
// the bars move smoothly instead of in whole rows.
var barGlyphs = [9]string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

func visTick() tea.Cmd {
	return tea.Tick(visFrame, func(time.Time) tea.Msg { return visTickMsg{} })
}

// visBands is how many bars fit a row of width w (centered).
func visBands(w int) int { return max((w-1)/(visBarW+1), 1) }

// updateBars advances the bar levels one frame with CAVA's two
// filters: an integral filter on rises (smooth, no jitter) and a
// gravity filter on falls (accelerating drop that reaches the floor).
func (m *Model) updateBars() {
	lo := m.layout()
	n := visBands(lo.W)
	if len(m.raw) != n {
		m.raw = make([]float64, n)
		m.bars = make([]float64, n)
		m.refreshVisColors()
	}
	if m.cur < 0 {
		m.fallBars(0) // nothing loaded: no bars at all
		return
	}
	if m.player.Paused() {
		m.fallBars(visFloor) // paused: settle on the resting row
		return
	}
	m.player.Bands(m.raw)
	for i, v := range m.raw {
		m.bars[i] = stepBar(m.bars[i], v)
	}
}

// refreshVisColors re-derives the bar gradient from the cover palette
// in use. Call it whenever either side changes — a new track's cover
// or a new bar count — or the bars keep wearing the previous song's
// colors.
func (m *Model) refreshVisColors() {
	m.visColors = gradient(m.artPal, len(m.bars))
}

// fallBars drops every bar toward level v (0 clears them, visFloor
// parks them while paused).
func (m *Model) fallBars(v float64) {
	for i := range m.bars {
		m.bars[i] = stepBar(m.bars[i], v)
	}
}

// stepBar is one filter step from cur toward v.
func stepBar(cur, v float64) float64 {
	if v > cur {
		return cur + (v-cur)*visIntegral
	}
	if cur = cur*visFall - visPull; cur < v {
		return v
	}
	return cur
}

// visRows renders the spectrum: exactly h rows of w cells, bars grown
// from the bottom with the bottom row kept for the song caption.
func (m Model) visRows(w, h int) []string {
	rows := make([]string, 0, h)
	if w <= 0 || h <= 0 {
		return rows
	}
	barsH := h
	caption := ""
	if h >= 3 {
		barsH = h - 2
		title, artist, _, _ := m.cardText()
		caption = title
		if artist != "" {
			caption += " — " + artist
		}
	}
	n := len(m.bars)
	cols := m.visColors
	if len(cols) != n {
		cols = gradient(m.artPal, n)
	}
	total := visBarW*n + max(n-1, 0)
	if total > w {
		// Layout changed and updateBars has not caught up yet: draw
		// only the bars that fit, never break the frame width.
		n = max((w-1)/(visBarW+1), 0)
		total = visBarW*n + max(n-1, 0)
	}
	indent := max((w-total)/2, 0)
	var sb strings.Builder
	for r := 0; r < barsH; r++ {
		sb.Reset()
		sb.WriteString(strings.Repeat(" ", indent))
		below := (barsH - 1 - r) * 8 // eighths of this cell that sit under it
		for i := 0; i < n; i++ {
			eighths := clamp(int(m.bars[i]*float64(barsH)*8+0.5)-below, 0, 8)
			if eighths == 0 {
				sb.WriteString(strings.Repeat(" ", visBarW))
			} else {
				c := cols[i]
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
				sb.WriteString(strings.Repeat(barGlyphs[eighths], visBarW))
			}
			if i < n-1 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString("\x1b[0m")
		line := sb.String()
		line += strings.Repeat(" ", max(w-indent-total, 0))
		rows = append(rows, line)
	}
	if barsH < h {
		// Blank spacer row, then the caption on the very last row, so
		// the title never sits flush against the bars.
		rows = append(rows, strings.Repeat(" ", w))
		rows = append(rows, centerCells(dimStyle.Render(truncCells(caption, w)), w))
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return rows
}
