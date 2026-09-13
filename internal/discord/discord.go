// Package discord pushes optional Discord Rich Presence updates over the
// local Discord client (IPC). All network/blocking work runs in
// background goroutines — the TUI never waits on Discord.
//
// Discord limitation: image keys must be Art Assets uploaded under the
// configured Application ID, so local song covers cannot be referenced.
// Both images use the app logo ("logo" asset); the hover text of the
// large image carries the current song + artist.
package discord

import (
	"sync"
	"time"

	"github.com/hugolgst/rich-go/client"
)

// Asset keys (upload assets/GoMusic-bg.png under these names in the
// Discord developer portal) and the GitHub button target.
const (
	LogoAsset = "logo"
	GitHubURL = "https://github.com/ifritov-dev/GoMusic"
)

// Settings mirrors config.DiscordConfig (kept local to avoid an import
// cycle towards internal/config).
type Settings struct {
	Enabled    bool
	ClientID   string
	Details    string
	ShowTime   bool
	ShowButton bool
}

// TrackInfo is a snapshot of what is playing right now.
type TrackInfo struct {
	Active   bool // a track is loaded
	Title    string
	Artist   string
	Length   time.Duration
	Position time.Duration
	Playing  bool // transport running (not paused)
}

// Manager holds presence state and (re)connects on demand. If Discord
// is absent or restarts, every call degrades to a silent no-op and the
// next Update retries the login.
type Manager struct {
	mu       sync.Mutex
	st       Settings
	loggedIn bool
	info     TrackInfo
}

// rich-go keeps global socket state without any synchronization, so
// every IPC call process-wide is serialized through this gate.
var ipcMu sync.Mutex

func New() *Manager { return &Manager{} }

// Configure applies new settings. Disabling logs out (clears presence);
// enabling pushes the last known track snapshot.
func (m *Manager) Configure(st Settings) {
	m.mu.Lock()
	m.st = st
	needLogout := !st.Enabled && m.loggedIn
	info := m.info
	m.mu.Unlock()

	if needLogout {
		go m.logout()
		return
	}
	if st.Enabled {
		go m.push(st, info)
	}
}

// Update publishes a new track snapshot (track change, pause/resume,
// seek). It never blocks the caller.
func (m *Manager) Update(info TrackInfo) {
	m.mu.Lock()
	m.info = info
	st := m.st
	m.mu.Unlock()

	if !st.Enabled {
		return
	}
	go m.push(st, info)
}

// Shutdown clears the presence. Call on quit; silent when never logged in.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	loggedIn := m.loggedIn
	m.mu.Unlock()
	if loggedIn {
		go m.logout()
	}
}

func (m *Manager) logout() {
	ipcMu.Lock()
	defer ipcMu.Unlock()
	safeLogout()
	m.mu.Lock()
	m.loggedIn = false
	m.mu.Unlock()
}

func (m *Manager) push(st Settings, info TrackInfo) {
	if st.ClientID == "" {
		return
	}
	ipcMu.Lock()
	defer ipcMu.Unlock()
	m.mu.Lock()
	loggedIn := m.loggedIn
	m.mu.Unlock()
	if !loggedIn {
		if err := client.Login(st.ClientID); err != nil {
			return // Discord absent — retry on the next Update
		}
		m.mu.Lock()
		m.loggedIn = true
		m.mu.Unlock()
	}
	if err := client.SetActivity(buildActivity(st, info)); err != nil {
		// Stale socket (Discord restarted?) — drop it; next Update redials.
		safeLogout()
		m.mu.Lock()
		m.loggedIn = false
		m.mu.Unlock()
	}
}

// rich-go panics when closing a dead socket; presence must never crash us.
func safeLogout() {
	defer func() { _ = recover() }()
	client.Logout()
}

func buildActivity(st Settings, info TrackInfo) client.Activity {
	song := info.Title
	if info.Artist != "" {
		song += " — " + info.Artist
	}
	if song == "" {
		song = "GoMusic"
	}
	act := client.Activity{
		Details:    firstNonEmpty(st.Details, "Using GoMusic"),
		LargeImage: LogoAsset,
		LargeText:  song,
		SmallImage: LogoAsset,
		SmallText:  "GoMusic",
	}
	if info.Active {
		if info.Playing {
			act.State = "Listening to " + song
			if st.ShowTime && info.Length > 0 {
				start := time.Now().Add(-info.Position)
				end := start.Add(info.Length)
				act.Timestamps = &client.Timestamps{Start: &start, End: &end}
			}
		} else {
			act.State = "Paused · " + song
		}
	} else {
		act.State = "Browsing library"
	}
	if st.ShowButton {
		act.Buttons = []*client.Button{{Label: "GoMusic on GitHub", Url: GitHubURL}}
	}
	return act
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
