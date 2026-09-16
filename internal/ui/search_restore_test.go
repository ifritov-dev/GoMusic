package ui

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ifritov-dev/GoMusic/internal/config"
	"github.com/ifritov-dev/GoMusic/internal/library"
	"github.com/ifritov-dev/GoMusic/internal/player"
)

func runeKey(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// updateSync feeds msg through Update and drains the resulting filter
// cmds (bubbles filtering is async: matches arrive as
// FilterMatchesMsg, which the real runtime delivers back).
func updateSync(m Model, msg tea.Msg) Model {
	upd, cmd := m.Update(msg)
	return drainCmd(upd.(Model), cmd)
}

func drainCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case list.FilterMatchesMsg:
		upd, next := m.Update(msg)
		return drainCmd(upd.(Model), next)
	case tea.BatchMsg:
		for _, c := range msg {
			m = drainCmd(m, c)
		}
		return m
	default:
		return m
	}
}

// f opens the library search: the list enters Filtering state.
func TestSearchOpensOnF(t *testing.T) {
	m := testModel(99, 31)
	if m.list.FilterState() != list.Unfiltered {
		t.Fatalf("filter must start off, got %v", m.list.FilterState())
	}
	upd, _ := m.Update(runeKey("f"))
	m = upd.(Model)
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("f must start filtering, got %v", m.list.FilterState())
	}
}

// Typing narrows by title/artist/album (app bindings like s/a/d must
// type text, not trigger while filtering), enter applies, esc clears.
func TestSearchFiltersAndClears(t *testing.T) {
	m := testModel(99, 31)
	m = updateSync(m, runeKey("f"))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("f must start filtering, got %v", m.list.FilterState())
	}
	for _, r := range []string{"s", "o", "n", "g", " ", "n", "a", "m", "e", " ", "3"} {
		m = updateSync(m, runeKey(r))
	}
	if got := len(m.list.VisibleItems()); got != 1 {
		t.Fatalf("visible after query = %d, want 1", got)
	}
	if sel, ok := m.list.SelectedItem().(trackItem); !ok || sel.t.Title != "song name 3" {
		t.Fatalf("match must be song name 3, got %+v", m.list.SelectedItem())
	}
	// Shuffle must not have toggled mid-query: "s" typed text.
	if m.cfg.Shuffle {
		t.Fatal("typing s in search must not toggle shuffle")
	}
	m = updateSync(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.list.FilterState() != list.FilterApplied {
		t.Fatalf("enter must apply filter, got %v", m.list.FilterState())
	}
	m = updateSync(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.list.FilterState() != list.Unfiltered {
		t.Fatalf("esc must clear filter, got %v", m.list.FilterState())
	}
	if got := len(m.list.VisibleItems()); got != len(m.tracks) {
		t.Fatalf("visible after clear = %d, want %d", got, len(m.tracks))
	}
}

// writeTestWav writes a stereo PCM16 WAV of silence.
func writeTestWav(t *testing.T, path string, secs float64) {
	t.Helper()
	const rate = 48000
	n := int(float64(rate) * secs)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+n*4))
	copy(hdr[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 2)
	binary.LittleEndian.PutUint32(hdr[24:], rate)
	binary.LittleEndian.PutUint32(hdr[28:], rate*4)
	binary.LittleEndian.PutUint16(hdr[32:], 4)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(n*4))
	if _, err := f.Write(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, n*4)); err != nil {
		t.Fatal(err)
	}
}

func restoreTestModel(t *testing.T, cfg *config.Config, tracks []library.Track) Model {
	t.Helper()
	p := player.New(100)
	p.Init()
	m := New(cfg, p)
	m.width, m.height = 99, 31
	m.tracks = tracks
	m.setItems()
	_ = m.relayout()
	return m
}

// The first library load resumes the saved track paused at the saved
// position; a refresh (`u`) must never hijack playback again.
func TestRestoreResumesSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir := t.TempDir()
	f1 := filepath.Join(dir, "one.wav")
	f2 := filepath.Join(dir, "two.wav")
	writeTestWav(t, f1, 2.0)
	writeTestWav(t, f2, 4.0)

	cfg := config.Default()
	cfg.LastTrack = f2
	cfg.LastPosSec = 1.0
	m := restoreTestModel(t, cfg, []library.Track{
		{Path: f1, Title: "one"},
		{Path: f2, Title: "two"},
	})
	upd, _ := m.Update(libraryMsg{tracks: m.tracks})
	m = upd.(Model)
	if m.cur != 1 {
		t.Fatalf("restore must select saved track: cur=%d", m.cur)
	}
	if !m.player.Paused() {
		t.Fatal("restored track must start paused")
	}
	if pos := m.player.Position(); pos < 900*time.Millisecond || pos > 1100*time.Millisecond {
		t.Fatalf("restored position = %v, want ~1s", pos)
	}
	// A saved position at the very end restarts the track instead of
	// instantly skipping to the next one.
	cfg2 := config.Default()
	cfg2.LastTrack = f1
	cfg2.LastPosSec = 100
	m2 := restoreTestModel(t, cfg2, []library.Track{{Path: f1, Title: "one"}})
	upd, _ = m2.Update(libraryMsg{tracks: m2.tracks})
	m2 = upd.(Model)
	if m2.cur != 0 || m2.player.Position() > 100*time.Millisecond {
		t.Fatalf("end position must restart: cur=%d pos=%v", m2.cur, m2.player.Position())
	}
}

func TestRestoreSkipsWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cfg := config.Default()
	cfg.LastTrack = filepath.Join(t.TempDir(), "gone.mp3")
	cfg.LastPosSec = 5
	m := restoreTestModel(t, cfg, []library.Track{{Path: "/x/a.mp3", Title: "a"}})
	upd, _ := m.Update(libraryMsg{tracks: m.tracks})
	m = upd.(Model)
	if m.cur != -1 || m.playing {
		t.Fatalf("missing track must stay stopped: cur=%d playing=%v", m.cur, m.playing)
	}
	if !m.restored {
		t.Fatal("restore flag must be set even when nothing matched")
	}
}
