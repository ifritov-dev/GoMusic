package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func visKey(t *testing.T, m Model) Model {
	t.Helper()
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = mm.(Model)
	if !m.vis {
		t.Fatal("v did not turn the visualizer on")
	}
	if cmd == nil {
		t.Fatal("turning the visualizer on must start the animation tick")
	}
	return m
}

// TestVisualizerFrame keeps the exact-frame invariant with the bars
// on, at every arrangement (wide / stacked / tiny) and off again.
func TestVisualizerFrame(t *testing.T) {
	for _, wh := range [][2]int{{99, 31}, {70, 22}, {48, 16}, {200, 50}} {
		w, h := wh[0], wh[1]
		m := visKey(t, testModel(w, h))
		if len(m.bars) != visBands(w) {
			t.Fatalf("%dx%d: %d bars, want %d", w, h, len(m.bars), visBands(w))
		}
		for i := range m.bars {
			m.bars[i] = 0.7
		}
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) != h {
			t.Fatalf("%dx%d: got %d lines, want %d", w, h, len(lines), h)
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != w {
				t.Fatalf("%dx%d line %d width %d, want %d: %q", w, h, i, got, w, ln)
			}
		}
		if !strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
			t.Fatalf("%dx%d: no bars rendered", w, h)
		}
		plain := stripANSI(out)
		if !strings.Contains(plain, "song name 3") {
			t.Fatalf("%dx%d: song caption missing", w, h)
		}
		// List rows carry the album subtitle; the bars replace them.
		if strings.Contains(plain, "artist - album") {
			t.Fatalf("%dx%d: track list still visible under the bars", w, h)
		}

		// Off again: the list and card come back.
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
		m = mm.(Model)
		if m.vis || !strings.Contains(stripANSI(m.View()), "artist - album") {
			t.Fatalf("%dx%d: v did not restore the list view", w, h)
		}
	}
}

// TestBarsFall checks the one thing a visualizer must never do: keep
// bars up when there is no audio.
func TestBarsFall(t *testing.T) {
	m := visKey(t, testModel(99, 31))
	for i := range m.bars {
		m.bars[i] = 1
	}
	m.cur = -1 // nothing playing
	for i := 0; i < 40; i++ {
		m.updateBars()
	}
	for i, v := range m.bars {
		if v > 0.01 {
			t.Fatalf("bar %d stuck at %v", i, v)
		}
	}
	// A resize re-fits the bar count.
	m.width = 60
	m.updateBars()
	if len(m.bars) != visBands(60) {
		t.Fatalf("resize left %d bars, want %d", len(m.bars), visBands(60))
	}
}

// TestVisualizerCoverColors checks the bars wear the cover palette,
// not the theme.
func TestVisualizerCoverColors(t *testing.T) {
	m := testModel(99, 31)
	m.artPal = []color.RGBA{{200, 20, 40, 255}, {20, 120, 240, 255}}
	m = visKey(t, m)
	for i := range m.bars {
		m.bars[i] = 1
	}
	out := m.View()
	if !strings.Contains(out, "38;2;200;20;40") {
		t.Fatal("gradient start is not the cover color")
	}
	if !strings.Contains(out, "38;2;20;120;240") {
		t.Fatal("gradient end is not the cover color")
	}
}

func TestPaletteFromCover(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	red, blue := color.RGBA{220, 40, 60, 255}, color.RGBA{40, 90, 230, 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				img.Set(x, y, red)
			} else {
				img.Set(x, y, blue)
			}
		}
	}
	pal := paletteOf(img)
	if len(pal) != 2 {
		t.Fatalf("got %d colors, want the cover's red + blue", len(pal))
	}
	if luma(pal[0]) > luma(pal[1]) {
		t.Fatalf("palette must run dark -> light: %v", pal)
	}
	if pal[0].B <= pal[0].R || pal[1].R <= pal[1].B {
		t.Fatalf("hues lost: %v", pal)
	}

	gray := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			gray.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}
	if got := paletteOf(gray); got != nil {
		t.Fatalf("gray cover produced a palette: %v", got)
	}
	if got := paletteOf(nil); got != nil {
		t.Fatalf("nil cover produced a palette: %v", got)
	}

	// B&W cover: darkest and lightest should be returned.
	bw := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				bw.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				bw.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	pal = paletteOf(bw)
	if len(pal) != 2 {
		t.Fatalf("B&W cover got %d colors, want 2", len(pal))
	}
	if luma(pal[0]) >= luma(pal[1]) {
		t.Fatalf("B&W palette not dark->light: %v", pal)
	}

	// Gradients must always fill, including the degenerate sizes.
	if got := gradient(nil, 5); len(got) != 5 {
		t.Fatalf("fallback gradient has %d colors", len(got))
	}
	if got := gradient([]color.RGBA{red}, 3); len(got) != 3 || got[0] != red || got[2] != red {
		t.Fatalf("single-color gradient wrong: %v", got)
	}
	if got := gradient(pal, 0); got != nil {
		t.Fatalf("zero-width gradient returned %d colors", len(got))
	}
	if got := gradient(pal, 1); len(got) != 1 {
		t.Fatalf("one-bar gradient returned %d colors", len(got))
	}
}

// TestVisualizerKeepsOneTickChain: spamming `v` must not stack
// animation chains (each would add another 20 fps of work).
func TestVisualizerKeepsOneTickChain(t *testing.T) {
	vKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}
	m := visKey(t, testModel(99, 31))
	if !m.visTicking {
		t.Fatal("first toggle must arm the animation")
	}
	mm, _ := m.Update(vKey)
	m = mm.(Model)
	mm, cmd := m.Update(vKey) // off and straight back on
	m = mm.(Model)
	if cmd != nil {
		t.Fatal("re-enabling armed a second tick chain")
	}
	// The chain that was in flight keeps driving the bars.
	mm, cmd = m.Update(visTickMsg{})
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("the live chain stopped animating")
	}
	// And it stops for good once a tick lands while the bars are off.
	mm, _ = m.Update(vKey)
	m = mm.(Model)
	mm, cmd = m.Update(visTickMsg{})
	m = mm.(Model)
	if cmd != nil || m.visTicking {
		t.Fatal("animation chain kept running after the visualizer closed")
	}
}

// TestVisualizerLastBarRenders pins the rightmost bar: a dead top band
// made it look like the last bar was never drawn, and an off-by-one in
// the bar pitch would do the same. Only the last bar is lit here, so
// every glyph in the frame belongs to it.
func TestVisualizerLastBarRenders(t *testing.T) {
	const w, h = 99, 31
	m := visKey(t, testModel(w, h))
	n := len(m.bars)
	for i := range m.bars {
		m.bars[i] = 0
	}
	m.bars[n-1] = 1
	out := m.View()
	lines := strings.Split(out, "\n")

	barsH := m.layout().midH - 2 // last row before the gap + caption
	row := []rune(stripANSI(lines[2+barsH-1]))
	total := visBarW*n + (n - 1)
	indent := (w - total) / 2
	want := indent + (n-1)*(visBarW+1)
	if got := strings.Index(string(row), "██"); got != want {
		t.Fatalf("last bar starts at column %d, want %d: %q", got, want, string(row))
	}
	if !strings.Contains(strings.Repeat("█", visBarW), strings.Repeat("█", visBarW)) {
		t.Fatal("unreachable")
	}
	if strings.Count(string(row), "█") != visBarW {
		t.Fatalf("last bar is %d cells wide, want %d: %q", strings.Count(string(row), "█"), visBarW, string(row))
	}
}

// TestBarsSmooth covers the CAVA-style filters: an integral filter on
// rises (no single-frame snapping) and a gravity filter on falls that
// lands the bar at rest.
func TestBarsSmooth(t *testing.T) {
	if got := stepBar(0, 1); got <= 0 || got >= 1 {
		t.Fatalf("first frame jumped to %v, want a partial step", got)
	}
	cur := 0.0
	for i := 0; i < 6; i++ {
		cur = stepBar(cur, 1)
	}
	if cur < 0.9 {
		t.Fatalf("sustained level only reaches %v", cur)
	}
	if got := stepBar(1, 0); got <= 0.5 || got >= 1 {
		t.Fatalf("bar fell to %v in a single frame", got)
	}
	// Gravity must land the bar exactly at rest, not creep forever.
	cur, frames := 1.0, 0
	for cur > 0 && frames < 500 {
		cur, frames = stepBar(cur, 0), frames+1
	}
	if cur != 0 {
		t.Fatalf("bars never land at 0 (stuck at %v)", cur)
	}
	// A quieter frame pulls the bar down to it, never below.
	if got := stepBar(0.5, 0.4); got < 0.4 {
		t.Fatalf("bar undershot its signal: %v", got)
	}
}

// TestBarsRestOnPause: a paused track keeps a low row of bars instead
// of an empty screen, and stops falling at the floor.
func TestBarsRestOnPause(t *testing.T) {
	m := visKey(t, testModel(99, 31))
	for i := range m.bars {
		m.bars[i] = 1
	}
	m.player.SetPaused(true)
	for i := 0; i < 60; i++ {
		m.updateBars()
	}
	for i, v := range m.bars {
		if v != visFloor {
			t.Fatalf("bar %d rests at %v, want the %v floor", i, v, visFloor)
		}
	}
	if !strings.ContainsAny(stripANSI(m.View()), "▁▂▃▄▅▆▇█") {
		t.Fatal("paused bars are invisible: the floor draws nothing")
	}
	// Resuming with no audio (test player has no tap) falls back to 0,
	// and stopping clears them outright.
	m.player.SetPaused(false)
	for i := 0; i < 30; i++ {
		m.updateBars()
	}
	if m.bars[0] != 0 {
		t.Fatalf("bar stayed at %v after pause with no signal", m.bars[0])
	}
	for i := range m.bars {
		m.bars[i] = 1
	}
	m.cur = -1
	for i := 0; i < 60; i++ {
		m.updateBars()
	}
	if m.bars[0] != 0 {
		t.Fatalf("bar %v left standing with nothing loaded", m.bars[0])
	}
}

// TestPaletteFollowsTrack is the regression guard for "it took the
// colors from one song and never changed them": the bar gradient was
// only rebuilt when the bar *count* changed, so a new cover palette
// was ignored whenever the window stayed the same size.
func TestPaletteFollowsTrack(t *testing.T) {
	m := visKey(t, testModel(99, 31))
	for i := range m.bars {
		m.bars[i] = 1
	}
	lo := m.layout()
	red := color.RGBA{200, 20, 40, 255}
	blue := color.RGBA{20, 120, 240, 255}
	green := color.RGBA{20, 200, 90, 255}

	// Two consecutive tracks, same window: each cover's palette must
	// reach the bars.
	paint := func(pal []color.RGBA) string {
		mm, _ := m.Update(artMsg{
			path: m.tracks[m.cur].Path, w: lo.artW, h: lo.artH,
			art: placeholderArt(lo.artW, lo.artH), pal: pal,
		})
		m = mm.(Model)
		return m.View()
	}
	out := paint([]color.RGBA{red, blue})
	if !strings.Contains(out, "38;2;200;20;40") || !strings.Contains(out, "38;2;20;120;240") {
		t.Fatal("first cover's palette never reached the bars")
	}
	out = paint([]color.RGBA{green})
	if strings.Contains(out, "38;2;200;20;40") {
		t.Fatal("bars kept the previous song's color")
	}
	if !strings.Contains(out, "38;2;20;200;90") {
		t.Fatal("new cover's palette never reached the bars")
	}
	// A cover without usable color falls back to the theme pair.
	out = paint(nil)
	for _, c := range []string{"38;2;200;20;40", "38;2;20;200;90"} {
		if strings.Contains(out, c) {
			t.Fatalf("stale bar color %s survived a palette-less cover", c)
		}
	}
	if !strings.Contains(out, "38;2;124;58;237") {
		t.Fatal("no theme fallback for a cover without color")
	}
}
