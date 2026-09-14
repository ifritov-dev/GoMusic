package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ifritov-dev/GoMusic/internal/downloader"
)

// checkTabFrame asserts the shared borderless tab frame: exactly H
// lines, every line exactly W cells, no box-drawing glyphs (the
// player style has no boxes anywhere).
func checkTabFrame(t *testing.T, m Model, out string) {
	t.Helper()
	W, H := m.width, m.height
	lines := strings.Split(out, "\n")
	if len(lines) != H {
		t.Fatalf("tab %dx%d: got %d lines, want %d", W, H, len(lines), H)
	}
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != W {
			t.Fatalf("tab %dx%d line %d width %d, want %d: %q", W, H, i, got, W, ln)
		}
	}
	if strings.ContainsAny(out, "┌┐└┘│") {
		t.Fatalf("tab %dx%d: frame contains border glyphs", W, H)
	}
	if !strings.HasPrefix(stripANSI(lines[0]), " GoMusic") {
		t.Fatalf("tab header must carry the brand indent: %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("line 1 must be blank breathing room, got: %q", lines[1])
	}
	vol := lines[H-1]
	if !strings.Contains(vol, "Volume:") || !strings.Contains(vol, "press k for keybinds") {
		t.Fatalf("tab footer must be the shared volume/hint line: %q", vol)
	}
}

func settingsModel(w, h int) Model {
	m := testModel(w, h)
	m.mode = modeSettings
	m.dirPath = "/home/ifritov/Music"
	m.dirNames = []string{"..", "Jazz", "Rock"}
	m.dirCursor = 1
	m.dirOffset = 0
	return m
}

func TestSettingsFrameExact(t *testing.T) {
	for _, wh := range [][2]int{{99, 31}, {70, 22}, {48, 16}, {120, 32}} {
		m := settingsModel(wh[0], wh[1])
		out := m.View()
		checkTabFrame(t, m, out)
		if !strings.Contains(out, "Settings") {
			t.Fatalf("%dx%d: settings title missing", wh[0], wh[1])
		}
		// One menu row per setting, all in a row. Discord is off
		// by default, so the GitHub button row hides; Client ID
		// and Show time are gone entirely.
		for _, want := range []string{"Music folder:", "Discord Rich Presence:"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%dx%d: menu row %q missing", wh[0], wh[1], want)
			}
		}
		for _, gone := range []string{"Client ID:", "Show time:", "GitHub button:"} {
			if strings.Contains(out, gone) {
				t.Fatalf("%dx%d: row %q must be gone/hidden", wh[0], wh[1], gone)
			}
		}
		// The track list must not leak into settings…
		for i := 1; i <= 8; i++ {
			if strings.Contains(out, "song name") {
				t.Fatalf("%dx%d: song list leaked into settings", wh[0], wh[1])
			}
		}
		if !strings.Contains(out, "enter change") {
			t.Fatalf("%dx%d: menu hint missing", wh[0], wh[1])
		}
	}
}

func TestSettingsMenuCursor(t *testing.T) {
	m := settingsModel(99, 31)
	m.cfg.Discord.Enabled = true
	m.setCursor = 2 // GitHub button row, visible only with Discord on
	out := m.View()
	checkTabFrame(t, m, out)
	if !strings.Contains(out, "GitHub button:") {
		t.Fatalf("github row must show with discord on: %q", out)
	}
	if !strings.Contains(stripANSI(out), "▸ GitHub button:") {
		t.Fatalf("cursor row missing: %q", out)
	}
}

func TestSettingsBrowseFrame(t *testing.T) {
	for _, wh := range [][2]int{{99, 31}, {70, 22}} {
		m := settingsModel(wh[0], wh[1])
		m.browsing = true
		out := m.View()
		checkTabFrame(t, m, out)
		if !strings.Contains(out, "Jazz/") || !strings.Contains(out, "Rock/") {
			t.Fatalf("%dx%d: directory rows missing", wh[0], wh[1])
		}
		if strings.Contains(out, "Discord Rich Presence:") {
			t.Fatalf("%dx%d: menu leaked into the folder browser", wh[0], wh[1])
		}
	}
}

func TestSettingsCursorAndScroll(t *testing.T) {
	m := settingsModel(99, 31)
	m.browsing = true
	m.dirNames = []string{"..", "a", "b", "c"}
	m.dirCursor = 3
	m.dirOffset = 0
	m.clampDirOffset()
	out := m.View()
	if !strings.Contains(stripANSI(out), "▸ c/") {
		t.Fatalf("cursor row missing: %q", out)
	}
}

func enterKey() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }

func TestSettingsActivate(t *testing.T) {
	// Arrows walk the menu…
	m := settingsModel(99, 31)
	if m.currentRow() != rowFolder {
		t.Fatalf("cursor starts at %d, want folder row", m.setCursor)
	}
	upd, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	m = upd.(Model)
	if m.currentRow() != rowDiscord {
		t.Fatalf("cursor at %d, want discord row", m.setCursor)
	}
	// …enter on a toggle flips it at once (and reveals the
	// GitHub button row)…
	upd, _ = m.updateSettings(enterKey())
	m = upd.(Model)
	if !m.cfg.Discord.Enabled {
		t.Fatal("enter on the discord row must toggle it")
	}
	if len(m.settingRows()) != 3 {
		t.Fatalf("github row must appear with discord on, rows=%v", m.settingRows())
	}
	// …toggling back off hides it and clamps the cursor.
	m.setCursor = 2
	upd, _ = m.updateSettings(tea.KeyMsg{Type: tea.KeyUp})
	m = upd.(Model) // cursor back on the discord row
	upd, _ = m.updateSettings(enterKey())
	m = upd.(Model)
	if m.cfg.Discord.Enabled || len(m.settingRows()) != 2 || m.setCursor != 1 {
		t.Fatalf("discord off must hide github + clamp cursor: enabled=%v rows=%v cursor=%d",
			m.cfg.Discord.Enabled, m.settingRows(), m.setCursor)
	}
	// …enter on the folder starts path edit, esc cancels it…
	m.setCursor = 0
	upd, _ = m.updateSettings(enterKey())
	m = upd.(Model)
	if !m.editingPath {
		t.Fatal("enter on the folder row must start path editing")
	}
	upd, _ = m.updateSettings(tea.KeyMsg{Type: tea.KeyEsc})
	m = upd.(Model)
	if m.editingPath {
		t.Fatal("esc must cancel path editing")
	}
	// …l on the folder opens the browser instead…
	upd, _ = m.updateSettings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = upd.(Model)
	if !m.browsing {
		t.Fatal("l on the folder row must open the browser")
	}
	// Wheel walks rows too.
	m.browsing = false
	upd, _ = m.updateSettings(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m = upd.(Model)
	if m.currentRow() != rowDiscord {
		t.Fatalf("wheel-down cursor at %d, want discord row", m.setCursor)
	}
}

func TestSettingsPastePath(t *testing.T) {
	m := settingsModel(99, 31)
	// Pasting a real dir + enter applies it like the browser would.
	m.editingPath = true
	m.pathInput.SetValue(t.TempDir())
	upd, _ := m.updateSettings(enterKey())
	m = upd.(Model)
	if m.mode != modeMain || m.editingPath {
		t.Fatalf("valid pasted path must apply: mode=%d editing=%v status=%q", m.mode, m.editingPath, m.status)
	}
	if m.cfg.MusicDir != m.pathInput.Value() {
		t.Fatalf("MusicDir = %q", m.cfg.MusicDir)
	}
	// A bogus path reports why and stays in edit mode.
	m2 := settingsModel(99, 31)
	m2.editingPath = true
	m2.pathInput.SetValue(filepath.Join(t.TempDir(), "nope"))
	upd, _ = m2.updateSettings(enterKey())
	m2 = upd.(Model)
	if !m2.editingPath || m2.mode != modeSettings || m2.status == "" {
		t.Fatalf("bogus path must stay + explain: editing=%v mode=%d status=%q", m2.editingPath, m2.mode, m2.status)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	quoted := `"` + t.TempDir() + `"`
	squoted := `'` + t.TempDir() + `'`
	for _, c := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", true},
		{"   ", "", true},
		{quoted, filepath.Clean(strings.Trim(quoted, `"'`)), false}, // Explorer "Copy as path"
		{squoted, filepath.Clean(strings.Trim(squoted, `"'`)), false},
		{"~/Music", filepath.Join(home, "Music"), false},
		{".", cwd, false},
	} {
		got, err := expandPath(c.in)
		if c.wantErr != (err != nil) {
			t.Fatalf("expandPath(%q) err=%v", c.in, err)
			continue
		}
		if err != nil {
			continue
		}
		if got != c.want {
			t.Fatalf("expandPath(%q) = %q, want %q", c.in, got, c.want)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("expandPath(%q) = %q, not absolute", c.in, got)
		}
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	if got, err := ensureDir(dir); err != nil || got != filepath.Clean(dir) {
		t.Fatalf("ensureDir(dir) = %q, %v", got, err)
	}
	file := filepath.Join(dir, "f.mp3")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureDir(file); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("ensureDir(file) = %v, want not-a-folder", err)
	}
	if _, err := ensureDir(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "no such folder") {
		t.Fatalf("ensureDir(missing) = %v, want no-such-folder", err)
	}
}

func TestParentDir(t *testing.T) {
	if p, ok := parentDir("/a/b"); !ok || p != "/a" {
		t.Fatalf("parentDir(/a/b) = %q,%v", p, ok)
	}
	if _, ok := parentDir("/"); ok {
		t.Fatal("root must have no parent")
	}
	if got := dirLabel(".."); got != ".."+string(filepath.Separator) {
		t.Fatalf("dirLabel(..) = %q", got)
	}
	if got := dirLabel("Jazz"); got != "Jazz"+string(filepath.Separator) {
		t.Fatalf("dirLabel(Jazz) = %q", got)
	}
}

func TestDirBrowserListsOnlyDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Songs and stray files must never show up in settings.
	for _, f := range []string{"track.mp3", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	msg := loadDirCmd(root)()
	dm, ok := msg.(dirMsg)
	if !ok {
		t.Fatalf("loadDirCmd returned %T", msg)
	}
	if dm.err != nil {
		t.Fatal(dm.err)
	}
	if len(dm.names) != 2 || dm.names[0] != ".." || dm.names[1] != "Sub" {
		t.Fatalf("dir entries = %v, want [.. Sub]", dm.names)
	}
}

func TestDownloadFrameExact(t *testing.T) {
	// Idle tab: hint + shared frame.
	m := testModel(99, 31)
	m.mode = modeDownload
	m.dlStatus = "paste a YouTube / YouTube Music link — playlists download whole"
	out := m.View()
	checkTabFrame(t, m, out)
	if !strings.Contains(out, "Download") || !strings.Contains(out, "playlists download whole") {
		t.Fatalf("download idle body wrong: %q", out)
	}

	// Checking phase (no snapshot yet): status line, no crash.
	m.dlBusy = true
	m.dlProg = nil
	m.dlStatus = "checking link…"
	out = m.View()
	checkTabFrame(t, m, out)
	if !strings.Contains(out, "checking link") {
		t.Fatalf("check phase must show the status: %q", out)
	}

	// Playlist fetch: position, title, bar, percent.
	m.dlProg = &downloader.Progress{Total: 12, Done: 2, Current: "Some Song", Percent: 50}
	out = m.View()
	checkTabFrame(t, m, out)
	for _, want := range []string{"Track 3 of 12", "Some Song", "%", "─"} {
		if !strings.Contains(out, want) {
			t.Fatalf("playlist progress missing %q in: %q", want, out)
		}
	}

	// Single video: title + bar, no track position.
	m.dlProg = &downloader.Progress{Total: 1, Done: 0, Current: "Lone Track", Percent: 25}
	out = m.View()
	checkTabFrame(t, m, out)
	if strings.Contains(out, "Track ") || !strings.Contains(out, "Lone Track") {
		t.Fatalf("single progress wrong: %q", out)
	}
}

func TestTabsHelpOverlay(t *testing.T) {
	for _, mode := range []mode{modeSettings, modeDownload} {
		m := testModel(99, 31)
		m.mode = mode
		m.showHelp = true
		out := m.View()
		checkTabFrame(t, m, out)
		if !strings.Contains(out, "keybinds") || !strings.Contains(out, "shuffle on/off") {
			t.Fatalf("mode %d: help overlay missing", mode)
		}
	}
}

func TestRootVolumesNonWindows(t *testing.T) {
	// Outside Windows there are no sibling roots to list.
	if vols := rootVolumes("/"); len(vols) != 0 {
		t.Fatalf("rootVolumes(/) = %v, want nil", vols)
	}
}
