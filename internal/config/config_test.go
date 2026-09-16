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
	cfg := &Config{MusicDir: "/tmp/mus", Volume: 42, Shuffle: true, Repeat: RepeatOne,
		LastTrack: "/tmp/mus/a.mp3", LastPosSec: 12.5}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := Load()
	if loaded.MusicDir != "/tmp/mus" || loaded.Volume != 42 || !loaded.Shuffle || loaded.Repeat != RepeatOne {
		t.Fatalf("roundtrip mismatch: %+v", loaded)
	}
	if loaded.LastTrack != "/tmp/mus/a.mp3" || loaded.LastPosSec != 12.5 {
		t.Fatalf("session state mismatch: %+v", loaded)
	}
	_ = os.Remove(filepath.Join(home, ".config", "gomusic", "config.json"))
}

func TestLoadDefaultsWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "cfg"))
	cfg := Load()
	if cfg.Volume != 80 || cfg.Repeat != RepeatOff || cfg.Shuffle {
		t.Fatalf("bad defaults: %+v", cfg)
	}
	if cfg.LastTrack != "" || cfg.LastPosSec != 0 {
		t.Fatalf("session state must default empty: %+v", cfg)
	}
}

func TestLoadClampsNegativePosition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cfg := &Config{MusicDir: "/tmp/mus", LastTrack: "/tmp/mus/a.mp3", LastPosSec: -3}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if loaded := Load(); loaded.LastPosSec != 0 {
		t.Fatalf("negative position must clamp to 0, got %v", loaded.LastPosSec)
	}
}
