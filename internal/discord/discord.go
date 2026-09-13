// Package discord pushes optional Discord Rich Presence updates over the
// local Discord client (IPC). All blocking work runs in background
// goroutines — the TUI never waits on Discord.
//
// The IPC client is implemented in-house (internal/discord/ipc) so it
// can find sandboxed Discord installs (Flatpak/Snap) that stock
// libraries miss: those hide the socket outside the standard runtime
// dir and hardcode UID 1000 / discord-ipc-0 only.
//
// Discord limitation: image keys must be Art Assets uploaded under the
// configured Application ID, so local song covers cannot be referenced.
// Both images use the app logo ("logo" asset); the hover text of the
// large image carries the current song + artist.
package discord

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/earendil/gomusic/internal/discord/ipc"
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

// Presence payload (Discord activity JSON).
type activity struct {
	Details    string              `json:"details,omitempty"`
	State      string              `json:"state,omitempty"`
	Assets     activityAssets      `json:"assets,omitempty"`
	Timestamps *activityTimestamps `json:"timestamps,omitempty"`
	Buttons    []*activityButton   `json:"buttons,omitempty"`
}

type activityAssets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type activityTimestamps struct {
	Start *uint64 `json:"start,omitempty"`
	End   *uint64 `json:"end,omitempty"`
}

type activityButton struct {
	Label string `json:"label,omitempty"`
	Url   string `json:"url,omitempty"`
}

// Manager holds presence state and (re)connects on demand. If Discord
// is absent or restarts, every call degrades to a silent no-op and the
// next Update retries the connection.
type Manager struct {
	mu   sync.Mutex
	st   Settings
	cli  *ipc.Client
	addr string // cached working socket
	info TrackInfo

	connected atomic.Bool
	lastErr   atomic.Value // string
}

func New() *Manager { return &Manager{} }

// Configure applies new settings. Disabling drops the connection
// (clears presence); enabling pushes the last known track snapshot.
func (m *Manager) Configure(st Settings) {
	m.mu.Lock()
	m.st = st
	needLogout := !st.Enabled && m.cli != nil
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

// Shutdown drops the connection. Call on quit; silent when never connected.
func (m *Manager) Shutdown() {
	go m.logout()
}

// Status reports the last known connection state for the settings UI.
func (m *Manager) Status() (connected bool, detail string) {
	if d, _ := m.lastErr.Load().(string); d != "" {
		return m.connected.Load(), d
	}
	return m.connected.Load(), ""
}

func (m *Manager) setStatus(ok bool, detail string) {
	m.connected.Store(ok)
	m.lastErr.Store(detail)
}

func (m *Manager) logout() {
	m.mu.Lock()
	cli := m.cli
	m.cli, m.addr = nil, ""
	m.mu.Unlock()
	if cli != nil {
		cli.Close()
	}
	m.setStatus(false, "")
}

func (m *Manager) push(st Settings, info TrackInfo) {
	if st.ClientID == "" {
		m.setStatus(false, "no Application ID set")
		return
	}
	m.mu.Lock()
	cli := m.cli
	m.mu.Unlock()
	if cli == nil {
		var addr string
		var err error
		cli, addr, err = connect(st.ClientID)
		if err != nil {
			m.setStatus(false, "Discord not found")
			return
		}
		m.mu.Lock()
		if m.cli != nil {
			// Another push connected first; drop ours.
			dup := cli
			cli = m.cli
			m.mu.Unlock()
			dup.Close()
		} else {
			m.cli, m.addr = cli, addr
			m.mu.Unlock()
		}
	}
	if err := cli.SetActivity("SET_ACTIVITY", map[string]any{
		"pid":      os.Getpid(),
		"activity": buildActivity(st, info),
	}); err != nil {
		cli.Close()
		m.mu.Lock()
		if m.cli == cli {
			m.cli, m.addr = nil, ""
		}
		m.mu.Unlock()
		m.setStatus(false, "connection lost")
		return
	}
	m.setStatus(true, "")
}

// connect tries every discovered socket until the handshake succeeds.
func connect(clientID string) (*ipc.Client, string, error) {
	for _, addr := range ipc.Candidates() {
		if cli, err := ipc.Dial(addr, clientID, 2*time.Second); err == nil {
			return cli, addr, nil
		}
	}
	return nil, "", errors.New("no discord socket answered")
}

func buildActivity(st Settings, info TrackInfo) activity {
	song := info.Title
	if info.Artist != "" {
		song += " — " + info.Artist
	}
	if song == "" {
		song = "GoMusic"
	}
	act := activity{
		Details: firstNonEmpty(st.Details, "Using GoMusic"),
		Assets: activityAssets{
			LargeImage: LogoAsset,
			LargeText:  song,
			SmallImage: LogoAsset,
			SmallText:  "GoMusic",
		},
	}
	if info.Active {
		if info.Playing {
			act.State = "Listening to " + song
			if st.ShowTime && info.Length > 0 {
				start := uint64(time.Now().Add(-info.Position).UnixNano() / 1e6)
				end := start + uint64(info.Length/1e6)
				act.Timestamps = &activityTimestamps{Start: &start, End: &end}
			}
		} else {
			act.State = "Paused · " + song
		}
	} else {
		act.State = "Browsing library"
	}
	if st.ShowButton {
		act.Buttons = []*activityButton{{Label: "GoMusic on GitHub", Url: GitHubURL}}
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
