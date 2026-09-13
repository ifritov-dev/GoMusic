package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cfg := &Config{MusicDir: "/tmp/mus", Volume: 42, Shuffle: true, Repeat: RepeatOne}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := Load()
	if loaded.MusicDir != "/tmp/mus" || loaded.Volume != 42 || !loaded.Shuffle || loaded.Repeat != RepeatOne {
		t.Fatalf("roundtrip mismatch: %+v", loaded)
	}
	_ = os.Remove(filepath.Join(home, ".config", "gomusic", "config.json"))
}

func TestArtworkBackfill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Missing key -> artwork on (stable logo by default).
	write(`{"music_dir": "/tmp/m", "discord": {"enabled": true}}`)
	if got := Load(); !got.Discord.ShowArtwork {
		t.Fatal("missing show_artwork should default on")
	}
	// Explicit false is respected.
	write(`{"music_dir": "/tmp/m", "discord": {"enabled": true, "show_artwork": false}}`)
	if got := Load(); got.Discord.ShowArtwork {
		t.Fatal("explicit show_artwork=false must win")
	}
}

func TestLoadDefaultsWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	cfg := Load()
	if cfg.Volume != 80 || cfg.Repeat != RepeatOff || cfg.Shuffle {
		t.Fatalf("bad defaults: %+v", cfg)
	}
}
