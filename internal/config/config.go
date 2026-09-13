package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RepeatMode controls what happens when a track ends.
type RepeatMode string

const (
	RepeatOff RepeatMode = "off"
	RepeatAll RepeatMode = "all"
	RepeatOne RepeatMode = "one"
)

// DiscordConfig holds the optional Discord Rich Presence settings.
// Discord only renders image keys uploaded as Art Assets under the
// Application ID below (see README), so both images use the app logo.
type DiscordConfig struct {
	Enabled    bool   `json:"enabled"`
	ClientID   string `json:"client_id"` // Discord Application ID
	Details    string `json:"details"`   // details line, e.g. "Using GoMusic"
	ShowTime   bool   `json:"show_time"` // playback progress via timestamps
	ShowButton bool   `json:"show_button"`
	// ShowArtwork references the pre-uploaded `logo` Art Asset. Leave
	// off unless the asset exists, or the card may render degraded.
	ShowArtwork bool `json:"show_artwork"`
}

// Config is persisted as JSON in the OS user config dir.
type Config struct {
	MusicDir string        `json:"music_dir"`
	Volume   int           `json:"volume"` // 0-100
	Shuffle  bool          `json:"shuffle"`
	Repeat   RepeatMode    `json:"repeat"`
	Discord  DiscordConfig `json:"discord"`
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	music := filepath.Join(home, "Music")
	if home == "" {
		music = "."
	}
	return &Config{MusicDir: music, Volume: 80, Shuffle: false, Repeat: RepeatOff,
		Discord: DiscordConfig{Details: "Using GoMusic", ShowTime: true, ShowButton: true}}
}

func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "gomusic", "config.json"), nil
}

func Load() *Config {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg
	}
	if loaded.Volume < 0 || loaded.Volume > 100 {
		loaded.Volume = 80
	}
	switch loaded.Repeat {
	case RepeatOff, RepeatAll, RepeatOne:
	default:
		loaded.Repeat = RepeatOff
	}
	if loaded.MusicDir == "" {
		loaded.MusicDir = cfg.MusicDir
	}
	if loaded.Discord.Details == "" {
		loaded.Discord.Details = "Using GoMusic"
	}
	return &loaded
}

func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
