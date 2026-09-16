package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ifritov-dev/GoMusic/internal/config"
)

func TestRuLayoutMap(t *testing.T) {
	pairs := map[rune]rune{
		'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'й': 'q', 'к': 'r',
		'р': 'h', 'л': 'k', 'с': 'c', 'я': 'z', 'м': 'v', 'ё': '`',
		'Ф': 'A', 'Ы': 'S', 'і': 's', 'ї': ']', 'є': '\'', 'ґ': '\\',
	}
	for ru, en := range pairs {
		got := normLayout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ru}})
		if string(got.Runes) != string(en) {
			t.Fatalf("normLayout(%q) = %q, want %q", ru, got.Runes, en)
		}
	}
	// Latin runes, digits, symbols and multi-rune pastes pass through.
	for _, r := range []rune{'s', '+', '-', '?', ' '} {
		got := normLayout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if len(got.Runes) != 1 || got.Runes[0] != r {
			t.Fatalf("normLayout(%q) must pass through, got %q", r, got.Runes)
		}
	}
	got := normLayout(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("фы")})
	if string(got.Runes) != "фы" {
		t.Fatalf("multi-rune paste must pass through, got %q", got.Runes)
	}
	if got := normLayout(tea.KeyMsg{Type: tea.KeyEnter}); got.Type != tea.KeyEnter {
		t.Fatal("non-rune keys must pass through")
	}
}

// Every main-mode hotkey fires from the Russian layout: ы→shuffle,
// к→repeat, с→settings, а→search, й→quit.
func TestHotkeysWorkInRussianLayout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := testModel(99, 31)
	upd, _ := m.Update(runeKey("ы"))
	m = upd.(Model)
	if !m.cfg.Shuffle {
		t.Fatal("ы (s) must toggle shuffle")
	}

	m = testModel(99, 31)
	upd, _ = m.Update(runeKey("к"))
	m = upd.(Model)
	if m.cfg.Repeat != config.RepeatAll {
		t.Fatalf("к (r) must cycle repeat, got %q", m.cfg.Repeat)
	}

	m = testModel(99, 31)
	upd, _ = m.Update(runeKey("с"))
	m = upd.(Model)
	if m.mode != modeSettings {
		t.Fatal("с (c) must open settings")
	}

	m = testModel(99, 31)
	m = updateSync(m, runeKey("а"))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("а (f) must open search, got %v", m.list.FilterState())
	}

	m = testModel(99, 31)
	_, cmd := m.Update(runeKey("й"))
	if cmd == nil {
		t.Fatal("й (q) must quit")
	}
}

// Typed search text keeps its runes: ы in the filter is ы, not s.
func TestFilterKeepsCyrillic(t *testing.T) {
	m := testModel(99, 31)
	m = updateSync(m, runeKey("f"))
	m = updateSync(m, runeKey("ы"))
	if m.list.FilterValue() != "ы" {
		t.Fatalf("filter input must keep Cyrillic, got %q", m.list.FilterValue())
	}
	if m.cfg.Shuffle {
		t.Fatal("typing in search must not toggle shuffle")
	}
}

func TestRepeatGlyphs(t *testing.T) {
	m := testModel(99, 31)
	m.cfg.Repeat = config.RepeatOne
	ctrl := stripANSI(m.controlsText(99))
	if !strings.Contains(ctrl, "⟳₁") {
		t.Fatalf("repeat-one must show ⟳₁, got %q", ctrl)
	}
	m.cfg.Repeat = config.RepeatAll
	ctrl = stripANSI(m.controlsText(99))
	if !strings.Contains(ctrl, "⟳") || strings.Contains(ctrl, "₁") {
		t.Fatalf("repeat-all must show plain ⟳, got %q", ctrl)
	}
	m.cfg.Repeat = config.RepeatOff
	ctrl = stripANSI(m.controlsText(99))
	if !strings.Contains(ctrl, "⟳") || strings.Contains(ctrl, "₁") {
		t.Fatalf("repeat-off must show plain ⟳, got %q", ctrl)
	}
}

// The ₁ subscript must measure exactly 1 cell: the frame stays exact.
func TestRepeatOneFrameExact(t *testing.T) {
	m := testModel(99, 31)
	m.cfg.Repeat = config.RepeatOne
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 31 {
		t.Fatalf("got %d lines, want 31", len(lines))
	}
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != 99 {
			t.Fatalf("line %d width %d, want 99: %q", i, got, ln)
		}
	}
}

// The shuffle glyph renders in both states.
func TestShuffleGlyphPresent(t *testing.T) {
	for _, on := range []bool{false, true} {
		m := testModel(99, 31)
		m.cfg.Shuffle = on
		if stripped := stripANSI(m.controlsText(99)); !strings.Contains(stripped, "⮂") {
			t.Fatalf("shuffle=%v must show ⮂: %q", on, stripped)
		}
	}
}

// m mutes and unmutes, keeping the volume level (RU ь works too).
func TestMuteToggles(t *testing.T) {
	m := testModel(99, 31)
	vol := m.player.Volume()
	upd, _ := m.Update(runeKey("m"))
	m = upd.(Model)
	if !m.player.Muted() {
		t.Fatal("m must mute")
	}
	upd, _ = m.Update(runeKey("ь"))
	m = upd.(Model)
	if m.player.Muted() {
		t.Fatal("ь (m) must unmute")
	}
	if m.player.Volume() != vol {
		t.Fatalf("volume must restore to %d, got %d", vol, m.player.Volume())
	}
}
