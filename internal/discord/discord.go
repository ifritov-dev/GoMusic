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
	// DefaultClientID is the official GoMusic Discord Application ID,
	// used when the settings field is empty so presence works with
	// zero setup; a custom ID in settings always wins.
	DefaultClientID = "1548671143009980418"
)

// EffectiveClientID prefers a custom ID, falls back to the built-in one.
func EffectiveClientID(custom string) string {
	return firstNonEmpty(custom, DefaultClientID)
}

// Settings mirrors config.DiscordConfig (kept local to avoid an import
// cycle towards internal/config).
type Settings struct {
	Enabled     bool
	ClientID    string
	Details     string
	ShowTime    bool
	ShowButton  bool
	ShowArtwork bool
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
	Name string `json:"name,omitempty"`
	// Type 2 makes Discord render "Listening to" instead of "Playing".
	Type       int                 `json:"type,omitempty"`
	Details    string              `json:"details,omitempty"`
	State      string              `json:"state,omitempty"`
	Assets     *activityAssets     `json:"assets,omitempty"`
	Timestamps *activityTimestamps `json:"timestamps,omitempty"`
	Buttons    []*activityButton   `json:"buttons,omitempty"`
}

type activityAssets struct {
	// No LargeText/SmallText: Discord clients may render hover texts
	// as extra rows, duplicating the song line. Images only.
	LargeImage string `json:"large_image,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
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
// scanCooldown bounds background work when Discord is absent: a failed
// full scan suspends rescanning for this long.
var scanCooldown = 30 * time.Second

type Manager struct {
	mu   sync.Mutex
	st   Settings
	cli  *ipc.Client
	addr string // cached working socket
	info TrackInfo
	gen  uint64 // snapshot generation; stale pushes drop themselves
	// nextScan suppresses rescans while Discord is known-absent.
	nextScan time.Time

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
	m.gen++
	gen := m.gen
	m.mu.Unlock()

	if needLogout {
		go m.logout()
		return
	}
	if st.Enabled {
		go m.push(st, info, gen)
	}
}

// Update publishes a new track snapshot (track change, pause/resume,
// seek). It never blocks the caller.
func (m *Manager) Update(info TrackInfo) {
	m.mu.Lock()
	m.info = info
	m.gen++
	gen, st := m.gen, m.st
	m.mu.Unlock()

	if !st.Enabled {
		return
	}
	go m.push(st, info, gen)
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

func (m *Manager) push(st Settings, info TrackInfo, gen uint64) {
	if EffectiveClientID(st.ClientID) == "" {
		m.setStatus(false, "no Application ID set")
		return
	}
	m.mu.Lock()
	if gen != m.gen {
		// Superseded before any IPC (collapses the startup herd).
		m.mu.Unlock()
		return
	}
	cli := m.cli
	if cli == nil && time.Now().Before(m.nextScan) {
		// Discord known-absent; skip the rescan entirely.
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if cli == nil {
		var addr string
		var err error
		cli, addr, err = connect(EffectiveClientID(st.ClientID))
		if err != nil {
			m.mu.Lock()
			m.nextScan = time.Now().Add(scanCooldown)
			m.mu.Unlock()
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
	m.mu.Lock()
	if gen != m.gen {
		// Superseded during dial; sending would flash stale presence.
		m.mu.Unlock()
		return
	}
	// Success clears any negative cache.
	m.nextScan = time.Time{}
	m.mu.Unlock()
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
	title := firstNonEmpty(info.Title, "GoMusic")
	artist := firstNonEmpty(info.Artist, "Unknown Artist")
	act := activity{Name: "GoMusic", Type: 2}
	if st.ShowArtwork {
		// Only reference the asset when it was actually uploaded;
		// dangling keys degrade the whole card (images AND buttons).
		act.Assets = &activityAssets{LargeImage: LogoAsset, SmallImage: LogoAsset}
	}
	if info.Active {
		// Main row: song title only; second row: artist.
		act.Details = truncRunes(title, 128)
		if !info.Playing {
			act.Details = truncRunes("Paused · "+title, 128)
		}
		act.State = truncRunes(artist, 128)
		if st.ShowTime && info.Playing && info.Length > 0 {
			start := uint64(time.Now().Add(-info.Position).UnixNano() / 1e6)
			end := start + uint64(info.Length/1e6)
			act.Timestamps = &activityTimestamps{Start: &start, End: &end}
		}
	} else {
		act.Details = "Browsing library"
	}
	if st.ShowButton {
		act.Buttons = []*activityButton{{Label: "GitHub", Url: GitHubURL}}
	}
	return act
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
