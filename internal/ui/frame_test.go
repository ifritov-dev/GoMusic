package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ifritov-dev/GoMusic/internal/config"
	"github.com/ifritov-dev/GoMusic/internal/library"
	"github.com/ifritov-dev/GoMusic/internal/player"
	"github.com/muesli/termenv"
)

// Force ANSI output: without a TTY lipgloss strips all styles, which
// would make style assertions (pills, resets) untestable.
func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func testModel(w, h int) Model {
	cfg := config.Default()
	cfg.MusicDir = "/home/ifritov/Music"
	p := player.New(100)
	m := New(cfg, p)
	m.width, m.height = w, h
	var tracks []library.Track
	for i := 1; i <= 8; i++ {
		tracks = append(tracks, library.Track{Path: fmt.Sprintf("/home/ifritov/Music/song%02d.mp3", i), Title: fmt.Sprintf("song name %d", i), Artist: "artist", Album: "album"})
	}
	m.tracks = tracks
	m.setItems()
	m.cur = 2
	m.playing = true
	m.list.Select(2)
	_ = m.relayout()
	return m
}

// checkFrame asserts the exact W×H borderless frame.
func checkFrame(t *testing.T, w, h int) Model {
	t.Helper()
	m := testModel(w, h)
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
	// Borderless: no box-drawing borders anywhere in the main view.
	// (The progress bar's ─ fillers are fine.)
	if strings.ContainsAny(out, "┌┐└┘│") {
		t.Fatalf("%dx%d: frame contains border glyphs", w, h)
	}
	return m
}

func TestExactFrame99x31(t *testing.T) {
	m := checkFrame(t, 99, 31)
	out := m.View()
	lines := strings.Split(out, "\n")
	// Header carries the same 1-cell indent as every other row, and a
	// blank line separates it from the track list.
	if !strings.HasPrefix(stripANSI(lines[0]), " GoMusic   Library: 8 tracks   /home/ifritov/Music") {
		t.Fatalf("header mismatch: %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("line 1 must be blank breathing room, got: %q", lines[1])
	}
	found := false
	for _, ln := range lines {
		if strings.Contains(ln, "» song name 3") {
			found = true
		}
	}
	if !found {
		t.Fatal("playing marker » missing")
	}
	// H=31 → midH=26: spacer 27, controls 28, spacer 29, volume 30.
	ctrl := lines[28]
	if !strings.Contains(ctrl, "⟳") || !strings.Contains(ctrl, "⇄") {
		t.Fatalf("controls icons missing: %q", ctrl)
	}
	if strings.Count(ctrl, "0:00") < 2 {
		t.Fatalf("controls must show elapsed AND total: %q", ctrl)
	}
	if strings.Contains(ctrl, "…") {
		t.Fatalf("controls line must never be …-truncated: %q", ctrl)
	}
	vol := lines[30]
	if !strings.Contains(vol, "Volume: 100% (+/- to change, Shift for precise)") || !strings.Contains(vol, "press k for keybinds") {
		t.Fatalf("volume mismatch: %q", vol)
	}
	if strings.Contains(vol, "…") {
		t.Fatalf("volume line must never be …-truncated: %q", vol)
	}
	// Cover art placeholder present (no real audio files in test).
	if !strings.Contains(out, "♪") {
		t.Fatal("cover placeholder ♪ missing")
	}
	// Song info is centered under the art: no info line may be wider
	// than the square art.
	lo := m.layout()
	if lo.showArt {
		_, artist, album, format := m.cardText()
		for _, s := range []string{artist, album, format} {
			if lipgloss.Width(s) > lo.artW {
				t.Fatalf("info line wider than art (%d > %d): %q", lipgloss.Width(s), lo.artW, s)
			}
		}
	}
}

func TestFrameWidths(t *testing.T) {
	for _, wh := range [][2]int{{200, 60}, {150, 40}, {120, 32}, {90, 24}, {80, 24}, {76, 22}, {70, 22}, {64, 40}, {60, 30}, {60, 20}, {54, 25}, {48, 16}, {40, 12}, {100, 14}, {30, 10}} {
		m := checkFrame(t, wh[0], wh[1])
		lo := m.layout()
		if lo.showArt && lo.artW != 2*lo.artH {
			t.Fatalf("%dx%d: art not square: %dx%d", wh[0], wh[1], lo.artW, lo.artH)
		}
	}
}

func TestCompactMiddle(t *testing.T) {
	// One track in a tall single-column window: content, exactly one
	// blank spacer, then controls — leftover rows hide below the
	// volume line instead of gaping under the last track.
	m := testModel(50, 20)
	m.tracks = m.tracks[:1]
	m.setItems()
	m.cur = 0
	m.list.Select(0)
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Fatalf("got %d lines, want 20", len(lines))
	}
	// header 0, blank 1, title 2, subtitle 3 (no trailing separator
	// after the final track), spacer 4, controls 5.
	if !strings.Contains(lines[3], "artist") {
		t.Fatalf("last track row wrong: %q", lines[3])
	}
	if strings.TrimSpace(lines[4]) != "" {
		t.Fatalf("want exactly one blank separator, got: %q", lines[4])
	}
	if !strings.Contains(lines[5], "⟳") {
		t.Fatalf("controls must follow the single spacer: %q", lines[5])
	}
	if !strings.Contains(lines[7], "Volume:") {
		t.Fatalf("volume must follow controls: %q", lines[7])
	}
	for i := 8; i < 20; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			t.Fatalf("line %d below volume must be blank filler: %q", i, lines[i])
		}
	}
	// Full library still fills the whole frame (no filler):
	// header 0, blank 1, middle 2..15, spacer 16, controls 17.
	full := testModel(50, 20)
	flines := strings.Split(full.View(), "\n")
	if !strings.Contains(flines[17], "⟳") {
		t.Fatalf("full list must keep controls at the bottom: %q", flines[17])
	}
}

func TestTruncCellsResetsStyle(t *testing.T) {
	// Cutting styled text must terminate the style: otherwise a
	// cropped cover-art background leaks onto padding and neighbours
	// on resize.
	styled := "\x1b[38;2;200;40;120m\x1b[48;2;40;20;60m" + strings.Repeat("▀", 40) + "\x1b[0m"
	out := truncCells(styled, 10)
	if got := lipgloss.Width(out); got > 10 {
		t.Fatalf("truncated width %d, want <= 10", got)
	}
	if !strings.Contains(out, "\x1b[0m") {
		t.Fatal("truncated styled text must end with a reset")
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("truncated text must end with …, got %q", out)
	}
}

func TestPlayingRowPill(t *testing.T) {
	// The playing track reads as a full-width bold pill (title) +
	// highlighted subtitle; other rows carry no background. At 99x31
	// all 8 tracks fit: tracks 0-1 take rows 0-5, playing track 2
	// takes rows 6 (title), 7 (subtitle), 8 (separator).
	m := testModel(99, 31)
	lo := m.layout()
	rows := m.leftRows(lo.leftW, lo.midUsed, lo.rowsPer, lo.showAll)
	if !strings.Contains(rows[6], "» song name 3") {
		t.Fatalf("playing title row wrong: %q", rows[6])
	}
	if !strings.Contains(rows[6], "48;") || !strings.Contains(rows[6], "\x1b[1;") {
		t.Fatalf("playing title lacks bold background pill: %q", rows[6])
	}
	if !strings.Contains(rows[7], "48;") {
		t.Fatalf("playing subtitle lacks background pill: %q", rows[7])
	}
	if strings.Contains(rows[0], "48;") {
		t.Fatalf("non-playing row must not have a background: %q", rows[0])
	}
}

// iconSGR returns the escape sequence opening the given icon.
func iconSGR(t *testing.T, line, icon string) string {
	t.Helper()
	i := strings.Index(line, icon)
	if i < 0 {
		t.Fatalf("icon %q missing in %q", icon, line)
	}
	esc := strings.LastIndex(line[:i], "\x1b[")
	if esc < 0 {
		return ""
	}
	return line[esc:i]
}

func TestFooterPills(t *testing.T) {
	m := testModel(99, 31)
	w := 99 - 1
	// Shuffle ON: highlighted + bold. Repeat ONE: highlighted + bold.
	m.cfg.Shuffle = true
	m.cfg.Repeat = config.RepeatOne
	line := m.controlsText(w)
	sh := iconSGR(t, line, "⇄")
	rep := iconSGR(t, line, "⟳")
	for name, sgr := range map[string]string{"shuffle": sh, "repeat-one": rep} {
		if !strings.Contains(sgr, "48;") {
			t.Fatalf("%s icon lacks background pill: %q", name, sgr)
		}
		if !strings.HasPrefix(sgr, "\x1b[1;") {
			t.Fatalf("%s icon lacks bold: %q", name, sgr)
		}
	}
	// Repeat ALL: highlighted, regular weight.
	m.cfg.Repeat = config.RepeatAll
	line = m.controlsText(w)
	rep = iconSGR(t, line, "⟳")
	if !strings.Contains(rep, "48;") {
		t.Fatalf("repeat-all icon lacks background pill: %q", rep)
	}
	if strings.HasPrefix(rep, "\x1b[1;") {
		t.Fatalf("repeat-all icon must not be bold: %q", rep)
	}
	// Everything OFF: no pills at all.
	m.cfg.Shuffle = false
	m.cfg.Repeat = config.RepeatOff
	line = m.controlsText(w)
	if strings.Contains(line, "48;") {
		t.Fatalf("inactive footer must have no background pills: %q", line)
	}
}

func TestHelpFrameExact(t *testing.T) {
	for _, wh := range [][2]int{{99, 31}, {70, 22}, {48, 16}} {
		m := testModel(wh[0], wh[1])
		m.showHelp = true
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) != wh[1] {
			t.Fatalf("%dx%d help: got %d lines", wh[0], wh[1], len(lines))
		}
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != wh[0] {
				t.Fatalf("%dx%d help line %d width %d: %q", wh[0], wh[1], i, got, ln)
			}
		}
	}
}
