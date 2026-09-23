package ui

import (
	"context"
	"fmt"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ifritov-dev/GoMusic/internal/config"
	"github.com/ifritov-dev/GoMusic/internal/discord"
	"github.com/ifritov-dev/GoMusic/internal/downloader"
	"github.com/ifritov-dev/GoMusic/internal/library"
	"github.com/ifritov-dev/GoMusic/internal/player"
)

// Modes
type mode int

const (
	modeMain mode = iota
	modeSettings
	modeDownload
)

// Messages
type tickMsg time.Time
type trackEndedMsg struct{ gen int }
type libraryMsg struct{ tracks []library.Track }
type errMsg struct{ err error }
type statusClearMsg struct{}
type artMsg struct {
	path string
	w, h int
	art  string
	pal  []color.RGBA // visualizer palette extracted from the cover
}

// Key bindings ----------------------------------------------------

type keyMap struct {
	PlayPause  key.Binding
	Play       key.Binding
	Next       key.Binding
	Prev       key.Binding
	VolUp      key.Binding
	VolDown    key.Binding
	Mute       key.Binding
	SeekFwd    key.Binding
	SeekBack   key.Binding
	Shuffle    key.Binding
	Repeat     key.Binding
	Refresh    key.Binding
	Download   key.Binding
	Visualizer key.Binding
	Settings   key.Binding
	ApplyDir   key.Binding
	Back       key.Binding
	Keybinds   key.Binding
	Quit       key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.PlayPause, k.Next, k.Prev, k.Keybinds, k.Settings, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.PlayPause, k.Play, k.Next, k.Prev},
		{k.Shuffle, k.Repeat, k.VolUp, k.VolDown, k.Mute},
		{k.SeekFwd, k.SeekBack, k.Refresh, k.Download},
		{k.Visualizer, k.Settings, k.Keybinds, k.Back, k.Quit},
	}
}

var keys = keyMap{
	PlayPause:  key.NewBinding(key.WithKeys(" ", "p"), key.WithHelp("space/p", "play/pause")),
	Play:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "play")),
	Next:       key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "next")),
	Prev:       key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "prev")),
	VolUp:      key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+", "vol+")),
	VolDown:    key.NewBinding(key.WithKeys("-", "_"), key.WithHelp("-", "vol-")),
	Mute:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mute")),
	SeekFwd:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "+5s")),
	SeekBack:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "-5s")),
	Shuffle:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shuffle")),
	Repeat:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeat")),
	Refresh:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "refresh")),
	Download:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "download")),
	Visualizer: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "visualizer")),
	Settings:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "settings")),
	ApplyDir:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "use folder")),
	Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Keybinds:   key.NewBinding(key.WithKeys("k", "?"), key.WithHelp("k", "keybinds")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ruLayout maps Cyrillic letters to the Latin letter on the same
// physical key (Russian ЙЦУКЕН plus Ukrainian extras), so hotkeys
// work in any keyboard layout. Case is preserved: shifted letters
// stay shifted and keep not matching, exactly like in EN.
var ruLayout = map[rune]rune{
	'ё': '`', 'Ё': '~',
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J', 'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.',
	'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M', 'Б': '<', 'Ю': '>',
	// Ukrainian letters sit on the same physical keys.
	'і': 's', 'І': 'S', 'ї': ']', 'Ї': '}', 'є': '\'', 'Є': '"', 'ґ': '\\', 'Ґ': '|',
}

// normLayout rewrites a single-rune Cyrillic keypress to the Latin
// letter on the same physical key. Only hotkey paths use it — text
// inputs (search filter, folder path, URL) always get raw runes.
func normLayout(msg tea.KeyMsg) tea.KeyMsg {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		if en, ok := ruLayout[msg.Runes[0]]; ok {
			msg.Runes = []rune{en}
		}
	}
	return msg
}

// List item -------------------------------------------------------

type trackItem struct {
	t       library.Track
	playing bool
}

func (i trackItem) FilterValue() string { return i.t.Title + " " + i.t.Artist + " " + i.t.Album }
func (i trackItem) Title() string {
	if i.playing {
		return "▶ " + i.t.Title
	}
	return i.t.Title
}
func (i trackItem) Description() string {
	if i.t.Artist != "" {
		if i.t.Album != "" {
			return fmt.Sprintf("%s — %s", i.t.Artist, i.t.Album)
		}
		return i.t.Artist
	}
	return i.t.Path
}

// Styles ----------------------------------------------------------

var (
	accent       = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"}
	accent2      = lipgloss.AdaptiveColor{Light: "#0EA5E9", Dark: "#22D3EE"}
	dimColor     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFF900"))
	nowStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"})
	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	badgeOn      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000")).Background(accent).Padding(0, 1)
	badgeOff     = lipgloss.NewStyle().Foreground(dimColor).Padding(0, 1)
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	fmtStyle     = lipgloss.NewStyle().Foreground(accent2)
	sectionStyle = lipgloss.NewStyle().Foreground(dimColor).Bold(true)
	// Active-state pills: an accent background so the state reads at a
	// glance. pillBold adds bold (playing track, shuffle ON, any
	// repeat); pillSoft stays regular weight (playing subtitle).
	pillFg   = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#14101F"}
	pillBold = lipgloss.NewStyle().Bold(true).Foreground(pillFg).Background(accent)
	pillSoft = lipgloss.NewStyle().Foreground(pillFg).Background(accent)
)

// Model -----------------------------------------------------------

type Model struct {
	cfg      *config.Config
	player   *player.Player
	tracks   []library.Track
	list     list.Model
	mode     mode
	width    int
	height   int
	cur      int // index into tracks, -1 = none
	playing  bool
	playGen  int  // increments on every play/stop; drops stale trackEndedMsg
	restored bool // previous session resumed once (first library load)
	// Shuffle state: a permutation of track indexes played in order.
	// Guarantees no repeats within a cycle — a track repeats only
	// after every other track has played (then a new cycle starts).
	shuffleOrder []int // permutation of [0,len(tracks)), valid when Shuffle on
	shufflePos   int   // position in shuffleOrder of cur; -1 = before start/none
	showHelp     bool
	status       string
	errText      string
	// Cover art cache (rendered cells for the current box size).
	artPath string
	artText string
	artW    int
	artH    int
	// Cover palette: colors the visualizer bars. Loaded with the art
	// (even when the art box itself is hidden) so the full-width bars
	// always wear the song's colors; nil falls back to the theme pair.
	artPal []color.RGBA
	// Visualizer ("v").
	vis        bool
	visTicking bool         // a visTick chain is alive (never start a second)
	bars       []float64    // smoothed levels, one per bar
	raw        []float64    // reused Bands output
	visColors  []color.RGBA // gradient across the current bar count
	// Discord Rich Presence (optional; all IPC off the main path).
	discord *discord.Manager
	// Ticks since start (presence re-pushes every ~30s).
	ticks int
	// Settings is one menu list: the cursor picks a row, enter
	// changes it. The folder row edits the path inline (paste a
	// path) or opens a directories-only browser (songs are never
	// listed); the GitHub button row only shows with Discord on.
	setCursor   int
	browsing    bool     // folder browser owns the keyboard
	editingPath bool     // folder path input owns the keyboard
	dirPath     string   // browser location (absolute, cleaned)
	dirNames    []string // sorted subdirs, ".." first when escapable
	dirCursor   int
	dirOffset   int
	dirErr      string
	pathInput   textinput.Model
	// YouTube download tab.
	urlInput textinput.Model
	dlStatus string
	dlBusy   bool
	dlGen    int
	dlCancel context.CancelFunc
	dlProg   *downloader.Progress // live playlist/single snapshot
}

// Settings menu rows, top to bottom. The GitHub button only
// appears while Discord Rich Presence is on (see settingRows).
type settingRow int

const (
	rowFolder settingRow = iota
	rowDiscord
	rowShowButton
)

func New(cfg *config.Config, p *player.Player) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(nil, delegate, 60, 15)
	l.Title = "Library"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	// Built-in fuzzy filter over title/artist/album (see FilterValue),
	// opened with "f". The list's own view is never rendered — only
	// VisibleItems/SelectedItem feed the custom frame.
	l.SetFilteringEnabled(true)
	l.KeyMap.Filter.SetKeys("f")
	// "k" opens keybinds (see footer) so the list must not steal it:
	// cursor moves on ↑/↓ only.
	l.KeyMap.CursorUp.SetKeys("up")
	l.KeyMap.CursorDown.SetKeys("down")
	l.KeyMap.PrevPage.SetKeys("pgup")
	l.KeyMap.NextPage.SetKeys("pgdown")

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "/home/user/Music"
	pi.CharLimit = 4096

	ui := textinput.New()
	ui.Prompt = ""
	ui.Placeholder = "https://music.youtube.com/… or youtube.com/…"
	ui.CharLimit = 2048

	dm := discord.New()
	dm.Configure(discordSettingsOf(cfg))

	dirPath := cfg.MusicDir
	if abs, err := filepath.Abs(dirPath); err == nil {
		dirPath = abs
	}
	return Model{
		cfg: cfg, player: p, list: l,
		mode: modeMain, cur: -1, shufflePos: -1,
		discord: dm, pathInput: pi, urlInput: ui,
		dirPath: dirPath,
	}
}

func discordSettingsOf(cfg *config.Config) discord.Settings {
	d := cfg.Discord
	return discord.Settings{
		Enabled: d.Enabled, ClientID: d.ClientID,
		ShowTime: d.ShowTime, ShowButton: d.ShowButton,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadLibrary(m.cfg.MusicDir),
		tick(),
		m.syncDiscordCmd(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) loadLibrary(dir string) tea.Cmd {
	return func() tea.Msg {
		tracks, err := library.Scan(dir)
		if err != nil {
			return errMsg{err}
		}
		return libraryMsg{tracks}
	}
}

// loadArtCmd decodes + renders cover art off the main path, plus the
// cover palette for the visualizer.
func loadArtCmd(path string, w, h int) tea.Cmd {
	return func() tea.Msg {
		img, err := library.Cover(path)
		if err != nil {
			return artMsg{path: path, w: w, h: h, art: ""}
		}
		return artMsg{path: path, w: w, h: h, art: renderCover(img, w, h), pal: paletteOf(img)}
	}
}

func (m Model) waitTrackEnd(gen int) tea.Cmd {
	done := m.player.Done()
	return func() tea.Msg {
		<-done
		return trackEndedMsg{gen: gen}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return statusClearMsg{} })
}

// Layout ----------------------------------------------------------

// layout is the single source of truth for the frame. The design is
// borderless (the mock's boxes only illustrated regions):
//
//	header / middle / spacer / controls / spacer / volume
//
// The middle section compacts to the content (midUsed <= midH): when
// the track list is shorter than the panel — e.g. after filtering —
// there is just the content, the spacer, then the controls; leftover
// rows become invisible filler below the volume line instead of a
// void between the last track and the controls.
// Three arrangements adapt to the window:
//
//	side    (wide):   track list + song card side by side, split by
//	                  a blank gap.
//	stacked (narrow): full-width list over a full-width card with
//	                  the cover on the left and info beside it,
//	                  separated by a blank line.
//	single  (tiny):   list only; playback state stays in the footer
//	                  so nothing is ever lost.
type layout struct {
	W, H    int
	midH    int
	arrange int
	// Side mode: content column widths (leftW + gap + rightW == W).
	leftW  int
	rightW int
	// Stacked mode: row split (listH + 1 + cardH == midH).
	listH int
	cardH int
	// Compaction: list density, whether every track fits on screen,
	// rows the list actually needs and middle rows actually rendered
	// (midUsed <= midH).
	rowsPer  int
	showAll  bool
	listNeed int
	midUsed  int
	// Cover art cells. Art renders with half-block "▀" glyphs where
	// one cell holds two vertical pixels, so visually SQUARE art needs
	// artW == 2*artH — layout() enforces that ratio and centers the
	// nested box instead of stretching it full-width.
	artW    int
	artH    int
	showArt bool
}

const (
	arrangeSide = iota
	arrangeStacked
	arrangeSingle
)

// Fixed frame overhead: header, blank under it, spacer, controls,
// spacer, volume.
const frameOverhead = 6

// Gap between the list and the song card in side mode.
const sideGap = 4

const (
	sideMinW   = 76 // below this width the columns stack vertically
	singleMaxW = 54 // below this width only the list is shown
	// Side-mode rows under the art: blank + title + artist +
	// album + format.
	sideTextH = 5
	minArtH   = 4
)

func (m Model) layout() layout {
	W, H := m.width, m.height
	if W <= 0 || H <= 0 {
		return layout{}
	}
	var lo layout
	lo.W, lo.H = W, H
	lo.midH = max(H-frameOverhead, 1)

	// Tiny window: list only.
	if W < singleMaxW || lo.midH < 8 {
		lo.arrange = arrangeSingle
		lo.leftW = W
		lo.rowsPer = rowsPerTrack(lo.midH)
		lo.listNeed, lo.showAll = neededRows(len(m.list.VisibleItems()), lo.midH, lo.rowsPer)
		lo.midUsed = min(lo.midH, lo.listNeed)
		return lo
	}

	// Narrow window: stack the list over the song card.
	if W < sideMinW {
		cardH := clamp(lo.midH*40/100, 7, 12)
		listH := lo.midH - 1 - cardH
		if listH < 3 {
			// Too short to stack: list only.
			lo.arrange = arrangeSingle
			lo.leftW = W
			return lo
		}
		lo.arrange = arrangeStacked
		lo.leftW = W
		lo.listH, lo.cardH = listH, cardH
		lo.rowsPer = rowsPerTrack(listH)
		lo.listNeed, lo.showAll = neededRows(len(m.list.VisibleItems()), listH, lo.rowsPer)
		lo.midUsed = min(lo.midH, lo.listNeed+1+cardH)
		// Square art beside the text: 1-cell margins, a 2-cell gap
		// and a readable text column.
		artH := min((W-22)/2, cardH)
		if artH >= minArtH && 2*artH >= 8 {
			lo.artW, lo.artH = 2*artH, artH
			lo.showArt = true
		}
		return lo
	}

	// Wide window: list + song card side by side. The card gets a
	// generous ~45% share so art and info breathe on big terminals;
	// the cap grows with the window so fullscreen art is ~1.5x the
	// old 56-cell max instead of staying small in a sea of space.
	lo.arrange = arrangeSide
	lo.rightW = clamp(W*45/100, 32, 84)
	lo.leftW = max(W-lo.rightW-sideGap, 16)
	lo.rightW = W - lo.leftW - sideGap
	// Square art: bound the width by the column, the height by the
	// text reservation, then lock artW == 2*artH.
	artH := min(lo.rightW/2, lo.midH-sideTextH)
	if artH >= minArtH {
		lo.artW, lo.artH = 2*artH, artH
		lo.showArt = true
	}
	lo.rowsPer = rowsPerTrack(lo.midH)
	lo.listNeed, lo.showAll = neededRows(len(m.list.VisibleItems()), lo.midH, lo.rowsPer)
	cardNeed := sideTextH - 1 // info lines without the art
	if lo.showArt {
		cardNeed = lo.artH + sideTextH
	}
	lo.midUsed = min(lo.midH, max(lo.listNeed, cardNeed))
	return lo
}

// rowsPerTrack picks list density from available height: 3
// rows/track normally, 2 without the blank separator, title-only
// when tiny.
func rowsPerTrack(h int) int {
	if h < 4 {
		return 1
	}
	if h < 9 {
		return 2
	}
	return 3
}

// neededRows returns the rows the track list actually needs, plus
// whether every track fits on screen. Fitting lists skip the blank
// separator after the final track, so a short list ends with content
// and exactly one spacer separates it from the controls.
// n == 0 still needs one row for the empty message.
func neededRows(n, h, rowsPer int) (need int, showAll bool) {
	if n == 0 {
		return 1, false
	}
	if n*rowsPer <= h {
		need = n * rowsPer
		if rowsPer >= 3 {
			need-- // no separator after the final track
		}
		return need, true
	}
	return h, false
}

// relayout recomputes sizes after a resize and reloads art when the
// art box changed. Returns a Cmd (possibly nil).
func (m *Model) relayout() tea.Cmd {
	lo := m.layout()
	m.clampDirOffset()
	m.list.SetSize(max(lo.leftW, 10), max(lo.midH, 1))
	if m.cur < 0 || !lo.showArt {
		m.artW, m.artH = lo.artW, lo.artH
		return nil
	}
	if lo.artW == m.artW && lo.artH == m.artH && m.artPath == m.tracks[m.cur].Path {
		return nil
	}
	m.artW, m.artH = lo.artW, lo.artH
	// Drop art rendered for the old box: cropping it to the new size
	// would flash a stretched cover for a frame. The placeholder
	// keeps the exact size until the reload lands.
	m.artText = ""
	return loadArtCmd(m.tracks[m.cur].Path, lo.artW, lo.artH)
}

// Playback helpers -------------------------------------------------

func (m *Model) markPlaying(old, new int) []tea.Cmd {
	var cmds []tea.Cmd
	if old >= 0 && old < len(m.tracks) {
		cmds = append(cmds, m.list.SetItem(old, trackItem{t: m.tracks[old]}))
	}
	if new >= 0 && new < len(m.tracks) {
		cmds = append(cmds, m.list.SetItem(new, trackItem{t: m.tracks[new], playing: true}))
	}
	return cmds
}

func (m *Model) playIndex(i int) tea.Cmd {
	if i < 0 || i >= len(m.tracks) {
		return nil
	}
	if err := m.player.PlayFile(m.tracks[i].Path); err != nil {
		m.errText = "cannot play: " + err.Error()
		return nil
	}
	// New generation: any in-flight waitTrackEnd from the previous
	// track becomes stale (PlayFile closes the old Done channel,
	// which would otherwise wake the old waiter and fire a bogus
	// extra advance — the "tracks spin in a circle" bug).
	m.playGen++
	gen := m.playGen
	cmds := m.markPlaying(m.cur, i)
	m.syncShuffleOnPlay(i)
	m.cur = i
	m.playing = true
	m.list.Select(i)
	m.errText = ""
	// Always decode the cover, even with the art box hidden in a tiny
	// window: the visualizer bars need the palette. With the box
	// hidden artW/artH are 0 and only the palette survives.
	lo := m.layout()
	m.artW, m.artH = lo.artW, lo.artH
	cmds = append(cmds, loadArtCmd(m.tracks[i].Path, lo.artW, lo.artH))
	cmds = append(cmds, m.waitTrackEnd(gen), m.pushPresenceCmd())
	m.saveState()
	return tea.Batch(cmds...)
}

func (m *Model) stopPlayback() tea.Cmd {
	cmds := m.markPlaying(m.cur, -1)
	m.playGen++ // invalidate pending trackEndedMsg
	m.player.Stop()
	m.playing = false
	m.cur = -1
	m.shuffleOrder = nil
	m.shufflePos = -1
	m.artPath, m.artText = "", ""
	m.artPal = nil
	m.refreshVisColors()
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Shuffle (no repeats within a cycle) ------------------------------
// When shuffle is on, shuffleOrder holds a random permutation of all
// track indexes and shufflePos points at the current track inside it.
// Next walks forward through the permutation, Prev walks back through
// history. A track can only repeat after every other track has played:
// at the end of the permutation a new shuffled cycle starts (repeat
// ALL) or the queue stops (repeat OFF).

func (m *Model) shuffleOrderValid() bool {
	n := len(m.tracks)
	if !m.cfg.Shuffle || n == 0 || len(m.shuffleOrder) != n {
		return false
	}
	seen := make([]bool, n)
	for _, idx := range m.shuffleOrder {
		if idx < 0 || idx >= n || seen[idx] {
			return false
		}
		seen[idx] = true
	}
	return true
}

// resetShuffle builds a fresh random permutation. first (when >= 0)
// is moved to the front so the current track never repeats
// immediately and toggling shuffle on mid-playback keeps playing.
func (m *Model) resetShuffle(first int) {
	n := len(m.tracks)
	if n == 0 {
		m.shuffleOrder = nil
		m.shufflePos = -1
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	rand.Shuffle(n, func(a, b int) { order[a], order[b] = order[b], order[a] })
	m.shuffleOrder = order
	m.shufflePos = -1
	if first >= 0 && first < n {
		for k, v := range order {
			if v == first {
				order[0], order[k] = order[k], order[0]
				break
			}
		}
		m.shufflePos = 0
	}
}

// ensureShuffle rebuilds a broken order and re-syncs the position
// with cur (e.g. after a rescan shifted indexes).
func (m *Model) ensureShuffle() {
	if !m.cfg.Shuffle {
		return
	}
	if !m.shuffleOrderValid() {
		m.resetShuffle(m.cur)
		return
	}
	if m.cur >= 0 {
		if m.shufflePos < 0 || m.shufflePos >= len(m.shuffleOrder) ||
			m.shuffleOrder[m.shufflePos] != m.cur {
			for k, v := range m.shuffleOrder {
				if v == m.cur {
					m.shufflePos = k
					break
				}
			}
		}
	} else if m.shufflePos != -1 {
		m.shufflePos = -1
	}
}

// syncShuffleOnPlay moves the shuffle position to i. Sequential
// next/prev steps just advance the position; a manual jump (enter on
// an arbitrary track) is spliced right after the current position so
// the already-played prefix stays played and every unplayed track —
// including the skipped ones — still plays before any repeat.
func (m *Model) syncShuffleOnPlay(i int) {
	if !m.cfg.Shuffle {
		return
	}
	if !m.shuffleOrderValid() || i < 0 || i >= len(m.tracks) {
		m.resetShuffle(i)
		return
	}
	// Sequential step forward/back: just move the cursor.
	if m.shufflePos >= 0 {
		if m.shufflePos+1 < len(m.shuffleOrder) && m.shuffleOrder[m.shufflePos+1] == i {
			m.shufflePos++
			return
		}
		if m.shufflePos-1 >= 0 && m.shuffleOrder[m.shufflePos-1] == i {
			m.shufflePos--
			return
		}
		if m.shuffleOrder[m.shufflePos] == i {
			return
		}
	}
	// Manual jump: pull i out of its old slot and splice it right
	// after the current position.
	order := m.shuffleOrder
	at := -1
	for k, v := range order {
		if v == i {
			at = k
			break
		}
	}
	if at < 0 {
		m.resetShuffle(i)
		return
	}
	order = append(order[:at], order[at+1:]...)
	insert := m.shufflePos + 1
	if insert < 0 {
		insert = 0
	}
	if insert > len(order) {
		insert = len(order)
	}
	order = append(order, 0)
	copy(order[insert+1:], order[insert:])
	order[insert] = i
	m.shuffleOrder = order
	m.shufflePos = insert
}

// remapShuffleAfterRescan rebuilds the permutation by path so a
// library refresh keeps the no-repeat guarantee: surviving tracks
// keep their relative order (played prefix stays played), new tracks
// are shuffled into the unplayed tail.
func (m *Model) remapShuffleAfterRescan(oldPaths []string) {
	if !m.cfg.Shuffle {
		m.shuffleOrder = nil
		m.shufflePos = -1
		return
	}
	newIdx := make(map[string]int, len(m.tracks))
	for k, t := range m.tracks {
		newIdx[t.Path] = k
	}
	var order []int
	if len(oldPaths) > 0 {
		seen := make(map[string]bool, len(oldPaths))
		for _, p := range oldPaths {
			if seen[p] {
				continue
			}
			seen[p] = true
			if k, ok := newIdx[p]; ok {
				order = append(order, k)
			}
		}
	}
	// New files: shuffle and append after the surviving prefix.
	inOrder := make([]bool, len(m.tracks))
	for _, k := range order {
		inOrder[k] = true
	}
	var fresh []int
	for k := range m.tracks {
		if !inOrder[k] {
			fresh = append(fresh, k)
		}
	}
	rand.Shuffle(len(fresh), func(a, b int) { fresh[a], fresh[b] = fresh[b], fresh[a] })
	order = append(order, fresh...)
	m.shuffleOrder = order
	m.shufflePos = -1
	if m.cur >= 0 {
		for k, v := range order {
			if v == m.cur {
				m.shufflePos = k
				break
			}
		}
		if m.shufflePos < 0 {
			// cur fell out of the order (shouldn't happen since
			// cur is remapped by path first): start fresh.
			m.resetShuffle(m.cur)
		}
	}
}

func (m *Model) nextIndex(step int) int {
	n := len(m.tracks)
	if n == 0 {
		return -1
	}
	if m.cfg.Shuffle && n > 1 {
		m.ensureShuffle()
		if step >= 0 {
			next := m.shufflePos + 1
			if next < len(m.shuffleOrder) {
				return m.shuffleOrder[next]
			}
			// End of the cycle: every track has played. Repeat
			// only now — with a brand-new shuffled order.
			if m.cfg.Repeat == config.RepeatAll {
				cur := m.cur
				m.resetShuffle(-1)
				// Don't play the same track twice in a row across
				// the cycle boundary.
				if cur >= 0 && len(m.shuffleOrder) > 1 && m.shuffleOrder[0] == cur {
					j := 1 + rand.Intn(len(m.shuffleOrder)-1)
					m.shuffleOrder[0], m.shuffleOrder[j] = m.shuffleOrder[j], m.shuffleOrder[0]
				}
				return m.shuffleOrder[0]
			}
			return -1
		}
		// Prev walks back through shuffle history.
		if m.shufflePos > 0 {
			return m.shuffleOrder[m.shufflePos-1]
		}
		if m.cur >= 0 {
			return m.cur
		}
		return -1
	}
	j := m.cur + step
	if j < 0 {
		j = 0
	}
	if j >= n {
		if m.cfg.Repeat == config.RepeatAll {
			j = 0
		} else {
			return -1
		}
	}
	return j
}

func (m *Model) onTrackEnded(gen int) tea.Cmd {
	// Stale waiter from a previous track (manual next/prev/enter
	// closed its Done channel): ignore, or every manual switch
	// cascades into another auto-advance and the queue spins.
	if gen != m.playGen {
		return nil
	}
	switch m.cfg.Repeat {
	case config.RepeatOne:
		return m.playIndex(m.cur)
	default:
		j := m.nextIndex(1)
		if j < 0 {
			m.player.Stop() // clear length/format/position state
			m.playing = false
			m.playGen++ // end-of-queue waiter is consumed
			cmds := m.markPlaying(m.cur, -1)
			m.cur = -1
			m.shuffleOrder = nil
			m.shufflePos = -1
			m.artPath, m.artText = "", ""
			m.status = "queue finished"
			cmds = append(cmds, clearStatusAfter(2*time.Second), m.pushPresenceCmd())
			return tea.Batch(cmds...)
		}
		return m.playIndex(j)
	}
}

func (m *Model) setItems() {
	items := make([]list.Item, len(m.tracks))
	for i, t := range m.tracks {
		items[i] = trackItem{t: t, playing: i == m.cur}
	}
	m.list.SetItems(items)
	m.list.Title = fmt.Sprintf("Library · %d tracks", len(m.tracks))
}

func (m *Model) persist() { _ = m.cfg.Save() }

// saveState snapshots the session (track + position; volume/shuffle/
// repeat already live in cfg) and persists it. Called on quit, on
// track change and periodically while playing, so even a kill loses
// at most ~30s of position.
func (m *Model) saveState() {
	if m.player != nil {
		m.cfg.LastTrack = m.player.Current()
		m.cfg.LastPosSec = 0
		if m.cfg.LastTrack != "" {
			m.cfg.LastPosSec = m.player.Position().Seconds()
		}
	}
	m.persist()
}

// maybeRestore resumes the previous session once: the saved track is
// loaded, seeked to the saved position and left paused. It runs only
// on the first library load after startup (never on `u` refresh) and
// only when nothing is already playing.
func (m *Model) maybeRestore() tea.Cmd {
	if m.cur >= 0 || m.cfg.LastTrack == "" || len(m.tracks) == 0 {
		return nil
	}
	idx := -1
	for i, t := range m.tracks {
		if t.Path == m.cfg.LastTrack {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	// Capture before playIndex: it snapshots state (saving the new
	// track at ~0) which would clobber the saved position below.
	savedPos := time.Duration(m.cfg.LastPosSec * float64(time.Second))
	cmd := m.playIndex(idx)
	if m.cur != idx {
		return cmd // play failed (see errText): stay stopped
	}
	if length := m.player.Length(); length > 0 {
		pos := savedPos
		if pos < 0 || pos >= length-2*time.Second {
			pos = 0 // fresh start instead of an instant track-end skip
		}
		m.player.SeekTo(pos)
	}
	m.player.SetPaused(true)
	return tea.Batch(cmd, m.pushPresenceCmd())
}

// presenceInfo snapshots playback for the Discord manager.
func (m Model) presenceInfo() discord.TrackInfo {
	info := discord.TrackInfo{}
	if m.cur >= 0 && m.cur < len(m.tracks) {
		t := m.tracks[m.cur]
		info.Active = true
		info.Title, info.Artist = t.Title, t.Artist
		info.Length = m.player.Length()
		info.Position = m.player.Position()
		info.Playing = m.playing && !m.player.Paused()
	}
	return info
}

// pushPresenceCmd publishes the current snapshot (never blocks: the
// manager dials Discord in a background goroutine).
func (m Model) pushPresenceCmd() tea.Cmd {
	info := m.presenceInfo()
	return func() tea.Msg { m.discord.Update(info); return nil }
}

// syncDiscordCmd re-applies settings, then publishes the snapshot.
func (m Model) syncDiscordCmd() tea.Cmd {
	st, info := discordSettingsOf(m.cfg), m.presenceInfo()
	return func() tea.Msg { m.discord.Configure(st); m.discord.Update(info); return nil }
}

func repeatLabel(r config.RepeatMode) string {
	switch r {
	case config.RepeatAll:
		return "ALL"
	case config.RepeatOne:
		return "ONE"
	default:
		return "OFF"
	}
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func formatLine(ext string, rateHz, ch int) string {
	if ext == "" {
		return "no audio"
	}
	khz := fmt.Sprintf("%g kHz", float64(rateHz)/1000)
	chName := fmt.Sprintf("%dch", ch)
	switch ch {
	case 1:
		chName = "Mono"
	case 2:
		chName = "Stereo"
	}
	return fmt.Sprintf("%s · %s · %s", ext, khz, chName)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func cycleRepeat(r config.RepeatMode) config.RepeatMode {
	switch r {
	case config.RepeatOff:
		return config.RepeatAll
	case config.RepeatAll:
		return config.RepeatOne
	default:
		return config.RepeatOff
	}
}

// Update ----------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.relayout()

	case libraryMsg:
		// A rescan (startup or `u`) remaps the playing track by path so
		// the pill, cursor and footer survive a refresh.
		curPath := ""
		if m.cur >= 0 && m.cur < len(m.tracks) {
			curPath = m.tracks[m.cur].Path
		}
		// Snapshot the shuffle cycle as paths so it survives the
		// index shift below.
		var oldShufflePaths []string
		if m.cfg.Shuffle && len(m.shuffleOrder) > 0 {
			for _, idx := range m.shuffleOrder {
				if idx >= 0 && idx < len(m.tracks) {
					oldShufflePaths = append(oldShufflePaths, m.tracks[idx].Path)
				}
			}
		}
		m.tracks = msg.tracks
		var resel []tea.Cmd
		if curPath != "" {
			found := -1
			for i, t := range m.tracks {
				if t.Path == curPath {
					found = i
					break
				}
			}
			if found >= 0 {
				m.cur = found
			} else {
				// Playing file vanished: stop so the footer never
				// desyncs from the list.
				resel = m.markPlaying(m.cur, -1)
				m.player.Stop()
				m.playing = false
				m.cur = -1
				m.artPath, m.artText = "", ""
				m.artPal = nil
				m.refreshVisColors()
				m.playGen++
			}
		}
		m.remapShuffleAfterRescan(oldShufflePaths)
		m.setItems()
		if !m.restored {
			m.restored = true
			if cmd := m.maybeRestore(); cmd != nil {
				resel = append(resel, cmd)
			}
		}
		if m.cur >= 0 {
			m.list.Select(m.cur)
		}
		if len(m.tracks) == 0 {
			m.status = "no music found — press c to choose a folder"
		} else {
			m.status = fmt.Sprintf("%d tracks", len(m.tracks))
		}
		return m, tea.Batch(append(resel, clearStatusAfter(3*time.Second))...)

	case errMsg:
		m.errText = msg.err.Error()
		return m, nil

	case statusClearMsg:
		m.status = ""
		return m, nil

	case tickMsg:
		m.ticks++
		if m.ticks%60 == 0 {
			if m.playing {
				m.saveState() // crash-safe position, ~30s stale at most
			}
			// Re-detect Discord started after us; refresh timestamps.
			return m, tea.Batch(tick(), m.pushPresenceCmd())
		}
		return m, tick()

	case visTickMsg:
		if !m.vis {
			m.visTicking = false
			return m, nil
		}
		m.updateBars()
		return m, visTick()

	case trackEndedMsg:
		if m.playing && m.mode == modeMain {
			return m, m.onTrackEnded(msg.gen)
		}
		return m, nil

	case artMsg:
		// Drop stale renders (track or box changed meanwhile).
		if m.cur < 0 || m.tracks[m.cur].Path != msg.path {
			return m, nil
		}
		lo := m.layout()
		if msg.w != lo.artW || msg.h != lo.artH {
			return m, nil
		}
		m.artPath = msg.path
		m.artPal = msg.pal
		m.refreshVisColors() // new song, new cover colors
		if msg.art == "" {
			m.artText = placeholderArt(msg.w, msg.h)
		} else {
			m.artText = msg.art
		}
		return m, nil

	case dirMsg:
		if msg.path != m.dirPath {
			return m, nil // stale (navigated away meanwhile)
		}
		if msg.err != nil {
			m.dirErr = msg.err.Error()
			m.dirNames = nil
			m.dirCursor, m.dirOffset = 0, 0
			return m, nil
		}
		m.dirErr = ""
		m.dirNames = msg.names
		m.dirCursor = clamp(m.dirCursor, 0, max(len(m.dirNames)-1, 0))
		m.clampDirOffset()
		return m, nil
	}

	if m.mode == modeSettings {
		return m.updateSettings(msg)
	}
	if m.mode == modeDownload {
		return m.updateDownload(msg)
	}
	return m.updateMain(msg)
}

func (m Model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While typing a search query every key belongs to the filter
	// input — including the app's own bindings (s, a, d, q, …).
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	// Hotkeys work in any keyboard layout (see normLayout). After the
	// filter bypass above, so typed search text keeps its runes.
	if km, ok := msg.(tea.KeyMsg); ok {
		msg = normLayout(km)
	}
	// Mouse wheel moves the list cursor one track per notch — the
	// cursor (accent-colored title, played with enter) follows, so the
	// top visible track can be selected by scrolling to it.
	if mmsg, ok := msg.(tea.MouseMsg); ok {
		switch mmsg.Button {
		case tea.MouseButtonWheelUp:
			m.list.CursorUp()
		case tea.MouseButtonWheelDown:
			m.list.CursorDown()
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.saveState()
			m.discord.Shutdown()
			return m, tea.Quit
		case key.Matches(msg, keys.Keybinds):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, keys.Back):
			// An applied filter clears first — esc is also the
			// list's ClearFilter, but our binding runs first.
			if m.list.FilterState() != list.Unfiltered {
				m.list.ResetFilter()
				return m, nil
			}
			// esc is also "close the visualizer" (same as v):
			// the in-flight tick chain notices vis == false and
			// stops on its own, like the v toggle-off path.
			if m.vis {
				m.vis = false
				return m, nil
			}
			// Already home: esc only closes the keybinds overlay.
			m.showHelp = false
			return m, nil
		case key.Matches(msg, keys.Download):
			m.mode = modeDownload
			m.urlInput.Focus()
			if !m.dlBusy {
				m.dlProg = nil
			}
			if _, err := downloader.Bins(); err != nil {
				m.dlStatus = err.Error()
			} else if m.dlStatus == "" {
				m.dlStatus = "paste a YouTube / YouTube Music link — playlists download whole"
			}
			return m, textinput.Blink
		case key.Matches(msg, keys.Settings):
			m.mode = modeSettings
			m.setCursor = 0
			m.browsing = false
			m.editingPath = false
			m.pathInput.Blur()
			m.resetDirBrowser(m.cfg.MusicDir)
			return m, nil
		case key.Matches(msg, keys.PlayPause):
			if m.cur < 0 {
				return m, m.playSelected()
			}
			m.player.Toggle()
			m.playing = !m.player.Paused()
			return m, m.pushPresenceCmd()
		case key.Matches(msg, keys.Play):
			return m, m.playSelected()
		case key.Matches(msg, keys.Next):
			j := m.nextIndex(1)
			if j >= 0 {
				return m, m.playIndex(j)
			}
			m.status = "end of queue (repeat is off)"
			return m, clearStatusAfter(2 * time.Second)
		case key.Matches(msg, keys.Prev):
			j := m.nextIndex(-1)
			if j >= 0 {
				return m, m.playIndex(j)
			}
			return m, nil
		case key.Matches(msg, keys.Shuffle):
			m.cfg.Shuffle = !m.cfg.Shuffle
			if m.cfg.Shuffle {
				// New cycle, current track first: it won't repeat
				// until everything else has played.
				m.resetShuffle(m.cur)
			} else {
				m.shuffleOrder = nil
				m.shufflePos = -1
			}
			m.status = "shuffle " + onOff(m.cfg.Shuffle)
			m.persist()
			return m, clearStatusAfter(2 * time.Second)
		case key.Matches(msg, keys.Repeat):
			m.cfg.Repeat = cycleRepeat(m.cfg.Repeat)
			m.status = "repeat " + repeatLabel(m.cfg.Repeat)
			m.persist()
			return m, clearStatusAfter(2 * time.Second)
		case key.Matches(msg, keys.VolUp):
			step := 5
			if msg.String() == "+" {
				step = 1 // shifted: precise
			}
			m.player.SetVolume(m.player.Volume() + step)
			m.cfg.Volume = m.player.Volume()
			m.persist()
			return m, nil
		case key.Matches(msg, keys.VolDown):
			step := 5
			if msg.String() == "_" {
				step = 1 // shifted: precise
			}
			m.player.SetVolume(m.player.Volume() - step)
			m.cfg.Volume = m.player.Volume()
			m.persist()
			return m, nil
		case key.Matches(msg, keys.Mute):
			m.player.ToggleMute() // volume level kept, restores on unmute
			return m, nil
		case key.Matches(msg, keys.SeekFwd):
			m.player.SeekBy(5 * time.Second)
			return m, m.pushPresenceCmd()
		case key.Matches(msg, keys.SeekBack):
			m.player.SeekBy(-5 * time.Second)
			return m, m.pushPresenceCmd()
		case key.Matches(msg, keys.Refresh):
			m.status = "refreshing library…"
			return m, m.loadLibrary(m.cfg.MusicDir)
		case key.Matches(msg, keys.Visualizer):
			m.vis = !m.vis
			if !m.vis {
				return m, nil
			}
			m.updateBars() // draw the first frame now, not 50ms later
			if m.visTicking {
				return m, nil // already animating: keep the one chain
			}
			m.visTicking = true
			return m, visTick()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// playSelected plays the cursor-selected track (the accent-colored
// title in the list, moved with ↑/↓ or the mouse wheel).
func (m *Model) playSelected() tea.Cmd {
	if sel, ok := m.list.SelectedItem().(trackItem); ok {
		for i, t := range m.tracks {
			if t.Path == sel.t.Path {
				return m.playIndex(i)
			}
		}
	}
	return nil
}

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	raw := msg // text inputs (folder path) get raw runes, never normalized
	if km, ok := msg.(tea.KeyMsg); ok {
		msg = normLayout(km)
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(km, keys.Quit) {
			m.saveState()
			m.discord.Shutdown()
			return m, tea.Quit
		}
		// The path input owns the keyboard: enter uses the typed
		// path, esc cancels, everything else types/pastes.
		if m.editingPath {
			if key.Matches(km, keys.Back) {
				m.editingPath = false
				m.pathInput.Blur()
				return m, nil
			}
			if km.Type == tea.KeyEnter {
				return m, m.applyPastedPath()
			}
			return m.updatePathInput(raw)
		}
		// The folder browser owns the keyboard while open.
		if m.browsing {
			switch {
			case key.Matches(km, keys.Back):
				m.browsing = false
				return m, nil
			case key.Matches(km, keys.Keybinds):
				m.showHelp = !m.showHelp
				return m, nil
			case key.Matches(km, keys.ApplyDir):
				return m, m.applyMusicDir(m.dirPath)
			case km.Type == tea.KeyUp:
				m.moveDirCursor(-1)
				return m, nil
			case km.Type == tea.KeyDown:
				m.moveDirCursor(1)
				return m, nil
			case km.Type == tea.KeyEnter, km.String() == "l":
				return m, m.enterDir()
			case km.String() == "h", km.Type == tea.KeyBackspace:
				return m, m.leaveDir()
			}
			return m, nil
		}
		// The menu list: arrows pick a row, enter changes it, l/→
		// on the folder row opens the folder browser.
		switch {
		case key.Matches(km, keys.Back):
			m.mode = modeMain
			return m, m.pushPresenceCmd()
		case key.Matches(km, keys.Keybinds):
			m.showHelp = !m.showHelp
			return m, nil
		case km.Type == tea.KeyUp:
			m.moveSetCursor(-1)
			return m, nil
		case km.Type == tea.KeyDown:
			m.moveSetCursor(1)
			return m, nil
		case km.Type == tea.KeyEnter:
			return m, m.activateSetting(false)
		case km.String() == "l", km.Type == tea.KeyRight:
			return m, m.activateSetting(true)
		}
	}
	// Mouse wheel picks rows like the main track list: one notch,
	// one row — in the menu and in the folder browser alike.
	if mm, ok := msg.(tea.MouseMsg); ok && !m.editingPath {
		switch mm.Button {
		case tea.MouseButtonWheelUp:
			if m.browsing {
				m.moveDirCursor(-1)
			} else {
				m.moveSetCursor(-1)
			}
		case tea.MouseButtonWheelDown:
			if m.browsing {
				m.moveDirCursor(1)
			} else {
				m.moveSetCursor(1)
			}
		}
		return m, nil
	}
	if m.editingPath {
		return m.updatePathInput(raw)
	}
	return m, nil
}

// settingRows is the visible menu: the GitHub button only makes
// sense with Rich Presence on, so it hides otherwise.
func (m Model) settingRows() []settingRow {
	rows := []settingRow{rowFolder, rowDiscord}
	if m.cfg.Discord.Enabled {
		rows = append(rows, rowShowButton)
	}
	return rows
}

func (m *Model) moveSetCursor(step int) {
	m.setCursor = clamp(m.setCursor+step, 0, len(m.settingRows())-1)
}

func (m Model) currentRow() settingRow {
	rows := m.settingRows()
	return rows[clamp(m.setCursor, 0, len(rows)-1)]
}

// activateSetting changes the cursor-picked row: toggles flip at
// once; the folder row edits the path inline (browse=true opens
// the folder browser instead, for picking by hand).
func (m *Model) activateSetting(browse bool) tea.Cmd {
	switch m.currentRow() {
	case rowFolder:
		if browse {
			m.browsing = true
			m.resetDirBrowser(m.cfg.MusicDir)
			return loadDirCmd(m.dirPath)
		}
		m.editingPath = true
		m.pathInput.SetValue(m.cfg.MusicDir)
		m.pathInput.Focus()
		return textinput.Blink
	case rowDiscord:
		m.cfg.Discord.Enabled = !m.cfg.Discord.Enabled
		m.persist()
		m.moveSetCursor(0) // rows may have shrunk (button hides)
		m.status = "discord: " + onOff(m.cfg.Discord.Enabled)
		return tea.Batch(m.syncDiscordCmd(), clearStatusAfter(2*time.Second))
	case rowShowButton:
		m.cfg.Discord.ShowButton = !m.cfg.Discord.ShowButton
		m.persist()
		return m.syncDiscordCmd()
	}
	return nil
}

// applyPastedPath validates the typed/pasted folder path and uses
// it, or reports why not and stays in edit mode.
func (m *Model) applyPastedPath() tea.Cmd {
	dir, err := ensureDir(m.pathInput.Value())
	if err != nil {
		m.status = err.Error()
		return clearStatusAfter(3 * time.Second)
	}
	return m.applyMusicDir(dir)
}

// applyMusicDir switches the library to dir from either the
// pasted path or the folder browser.
func (m *Model) applyMusicDir(dir string) tea.Cmd {
	m.cfg.MusicDir = dir
	m.persist()
	m.mode = modeMain
	m.browsing = false
	m.editingPath = false
	m.pathInput.Blur()
	stopCmd := m.stopPlayback()
	m.status = "library: " + dir
	return tea.Batch(stopCmd, m.loadLibrary(dir), clearStatusAfter(3*time.Second))
}

// updatePathInput routes messages to the folder path input.
func (m Model) updatePathInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	return m, cmd
}

// Directory browser (settings) --------------------------------------
// Only subdirectories are ever listed — songs stay out of settings.

type dirMsg struct {
	path  string
	names []string
	err   error
}

func loadDirCmd(path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(path)
		if err != nil {
			return dirMsg{path: path, err: err}
		}
		subs := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				subs = append(subs, e.Name())
			}
		}
		sort.Strings(subs)
		names := make([]string, 0, len(subs)+2)
		if _, ok := parentDir(path); ok {
			names = append(names, "..")
		} else {
			// Filesystem root: on Windows that is one drive
			// (C:\) with siblings (D:\) unreachable by "..",
			// so list every live drive for switching between them.
			names = append(names, rootVolumes(path)...)
		}
		return dirMsg{path: path, names: append(names, subs...)}
	}
}

// rootVolumes lists sibling roots at a filesystem root. Only
// Windows has several (drives); elsewhere it is always empty.
func rootVolumes(path string) []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	var vols []string
	cur := strings.ToUpper(filepath.VolumeName(filepath.Clean(path)))
	for c := 'A'; c <= 'Z'; c++ {
		vol := string(c) + `:\`
		if strings.ToUpper(vol[:2]) == cur {
			continue // already here, listed via its subdirs
		}
		if st, err := os.Stat(vol); err == nil && st.IsDir() {
			vols = append(vols, vol)
		}
	}
	return vols
}

// expandPath normalizes a pasted/typed folder path on every OS:
// surrounding spaces and Explorer-style quotes go away, ~ means
// home, relatives resolve against the working directory.
func expandPath(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	if s == "" {
		return "", fmt.Errorf("paste a folder path first")
	}
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("cannot resolve ~")
		}
		s = home + s[1:]
	}
	if !filepath.IsAbs(s) {
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", err
		}
		s = abs
	}
	return filepath.Clean(s), nil
}

// ensureDir expands raw and requires an existing directory.
func ensureDir(raw string) (string, error) {
	p, err := expandPath(raw)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("no such folder: %s", p)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a folder: %s", p)
	}
	return p, nil
}

// dirLabel renders a browser entry with the OS separator (drives
// and roots already end with one, so nothing is appended twice).
func dirLabel(name string) string {
	sep := string(filepath.Separator)
	if name == ".." || (!filepath.IsAbs(name) && !strings.HasSuffix(name, sep)) {
		return name + sep
	}
	return name
}

func parentDir(path string) (string, bool) {
	parent := filepath.Dir(filepath.Clean(path))
	if parent == filepath.Clean(path) {
		return "", false
	}
	return parent, true
}

func absDir(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func (m *Model) resetDirBrowser(path string) {
	m.dirPath = absDir(path)
	m.dirNames = nil
	m.dirCursor, m.dirOffset = 0, 0
	m.dirErr = ""
}

// browserH is the visible directory row count, derived from the
// window height with the same formula the folder browser view uses
// (heading + path + hints around it), so scrolling and rendering
// always agree.
func (m Model) browserH() int {
	room := max(m.height-4, 1)
	return max(room-3, 3)
}

func (m *Model) moveDirCursor(step int) {
	if len(m.dirNames) == 0 {
		return
	}
	m.dirCursor = clamp(m.dirCursor+step, 0, len(m.dirNames)-1)
	m.clampDirOffset()
}

func (m *Model) clampDirOffset() {
	h := m.browserH()
	if m.dirCursor < m.dirOffset {
		m.dirOffset = m.dirCursor
	}
	if m.dirCursor > m.dirOffset+h-1 {
		m.dirOffset = m.dirCursor - h + 1
	}
	m.dirOffset = max(m.dirOffset, 0)
}

func (m *Model) enterDir() tea.Cmd {
	if m.dirCursor < 0 || m.dirCursor >= len(m.dirNames) {
		return nil
	}
	name := m.dirNames[m.dirCursor]
	if name == ".." {
		return m.leaveDir()
	}
	if filepath.IsAbs(name) {
		m.dirPath = filepath.Clean(name) // another drive/root
	} else {
		m.dirPath = filepath.Join(m.dirPath, name)
	}
	m.dirNames = nil
	m.dirCursor, m.dirOffset = 0, 0
	m.dirErr = ""
	return loadDirCmd(m.dirPath)
}

func (m *Model) leaveDir() tea.Cmd {
	parent, ok := parentDir(m.dirPath)
	if !ok {
		return nil
	}
	m.dirPath = parent
	m.dirNames = nil
	m.dirCursor, m.dirOffset = 0, 0
	m.dirErr = ""
	return loadDirCmd(m.dirPath)
}

func (m Model) discordStatusLine() string {
	d := m.cfg.Discord
	if !d.Enabled {
		return dimStyle.Render("disabled")
	}
	if discord.EffectiveClientID(d.ClientID) == "" {
		return dimStyle.Render("needs an Application ID")
	}
	if ok, detail := m.discord.Status(); ok {
		return badgeOn.Render("CONNECTED")
	} else if detail != "" {
		return badgeOff.Render("OFFLINE") + dimStyle.Render("  ("+detail+")")
	} else {
		return badgeOff.Render("OFFLINE")
	}
}

// View ------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.mode == modeSettings {
		return m.viewSettings()
	}
	if m.mode == modeDownload {
		return m.viewDownload()
	}
	return m.viewMain()
}

func (m Model) viewMain() string {
	lo := m.layout()
	W := lo.W
	if W < 10 {
		return "loading…"
	}
	m.list.SetSize(max(lo.leftW, 10), max(lo.midH, 1))

	lines := make([]string, 0, lo.H)
	// Header with the same 1-cell indent as every other row, plus a
	// blank line breathing room before the track list.
	lines = append(lines, " "+padCells(m.headerText(), W-1))
	lines = append(lines, strings.Repeat(" ", W))
	if m.showHelp {
		lines = append(lines, m.helpFrame(lo)...)
	} else if m.vis {
		// The visualizer takes the whole middle section: no columns,
		// no compaction — just bars and the caption row.
		lines = append(lines, m.visRows(W, lo.midH)...)
	} else if lo.arrange == arrangeSingle {
		for _, r := range m.leftRows(lo.leftW, lo.midUsed, lo.rowsPer, lo.showAll) {
			lines = append(lines, r)
		}
	} else if lo.arrange == arrangeStacked {
		// List over song card, separated by a blank line.
		for _, r := range m.leftRows(W, lo.listNeed, lo.rowsPer, lo.showAll) {
			lines = append(lines, r)
		}
		lines = append(lines, strings.Repeat(" ", W))
		for _, r := range m.cardRows(lo) {
			lines = append(lines, r)
		}
	} else {
		left := m.leftRows(lo.leftW, lo.midUsed, lo.rowsPer, lo.showAll)
		right := m.rightRows(lo, lo.midUsed)
		gap := strings.Repeat(" ", sideGap)
		for i := 0; i < lo.midUsed; i++ {
			lines = append(lines, left[i]+gap+right[i])
		}
	}
	// Spacer / controls / spacer / volume, then invisible filler so
	// short content still ends at exactly H lines.
	lines = append(lines, strings.Repeat(" ", W))
	lines = append(lines, " "+padCells(m.controlsText(W-1), W-1))
	lines = append(lines, strings.Repeat(" ", W))
	lines = append(lines, " "+padCells(m.volumeText(W-1), W-1))
	for len(lines) < lo.H {
		lines = append(lines, strings.Repeat(" ", W))
	}
	// Errors/status surface inside the volume row via volumeText so
	// the frame stays exactly H lines.
	return strings.Join(lines, "\n")
}

// headerText is "GoMusic   Library: N tracks   <dir>", truncated to fit.
func (m Model) headerText() string {
	inner := max(m.width-2, 0)
	lib := fmt.Sprintf("Library: %d tracks", len(m.tracks))
	extra := ""
	switch m.list.FilterState() {
	case list.Filtering:
		extra = "   search: " + m.list.FilterValue()
	case list.FilterApplied:
		extra = fmt.Sprintf("   filter: %s (%d/%d, esc clears)",
			m.list.FilterValue(), len(m.list.VisibleItems()), len(m.tracks))
	}
	extra = truncCells(extra, max(inner/2, 0)) // the tag never eats the header
	full := "GoMusic   " + lib + "   " + m.cfg.MusicDir + extra
	if cellWidth(full) <= inner {
		return brandStyle.Render("GoMusic") + "   " + lib + "   " + dimStyle.Render(m.cfg.MusicDir) + dimStyle.Render(extra)
	}
	// Shrink the directory first, never the brand/counts.
	keep := "GoMusic   " + lib + "   "
	room := max(inner-cellWidth(keep)-cellWidth(extra), 0)
	return brandStyle.Render("GoMusic") + "   " + lib + "   " + dimStyle.Render(truncCells(m.cfg.MusicDir, room)) + dimStyle.Render(extra)
}

// leftRows renders the track list with a 1-cell leading indent. The
// playing track uses "» " on the title and aligns the subtitle
// under it with 3 leading spaces. Exactly h rows (blank-padded).
func (m Model) leftRows(w, h, rowsPer int, showAll bool) []string {
	rows := make([]string, 0, h)
	if w <= 0 || h <= 0 {
		return rows
	}
	items := m.list.VisibleItems()
	sel := m.list.Index()
	n := len(items)
	if n == 0 {
		msg := " no music — press c to choose a folder"
		if m.list.FilterState() != list.Unfiltered {
			msg = " no matches — esc to clear the search"
		}
		rows = append(rows, " "+padCells(truncCells(msg[1:], w-1), w-1))
		for len(rows) < h {
			rows = append(rows, strings.Repeat(" ", w))
		}
		return rows
	}
	// Fitting lists show every track (no separator after the final
	// one); longer lists window around the cursor.
	count := n
	start := 0
	if !showAll {
		count = max(h/rowsPer, 1)
		if sel >= 0 {
			start = clamp(sel-count/2, 0, max(n-count, 0))
		}
	}
	for g := 0; g < count && len(rows) < h; g++ {
		idx := start + g
		if idx >= n {
			break
		}
		ti, _ := items[idx].(trackItem)
		playing := m.cur >= 0 && m.cur < len(m.tracks) && m.tracks[m.cur].Path == ti.t.Path
		cursor := idx == sel
		title := ti.t.Title
		sub := trackSub(ti.t)
		var r1, r2 string
		if playing {
			r1 = " » " + truncCells(title, w-3)
			r2 = "   " + truncCells(sub, w-3)
		} else {
			r1 = " " + truncCells(title, w-1)
			r2 = " " + truncCells(sub, w-1)
		}
		if playing {
			// Full-width highlight pill, bold title: pad first so
			// the background spans the whole row, then paint.
			r1 = pillBold.Render(padCells(r1, w))
			r2 = pillSoft.Render(padCells(r2, w))
		} else if cursor {
			r1 = titleStyle.Render(r1)
			r2 = dimStyle.Render(r2)
		} else {
			r2 = dimStyle.Render(r2)
		}
		rows = append(rows, padCells(r1, w))
		if rowsPer >= 2 && len(rows) < h {
			rows = append(rows, padCells(r2, w))
		}
		lastShown := showAll && idx == n-1
		if rowsPer >= 3 && !lastShown && len(rows) < h {
			rows = append(rows, strings.Repeat(" ", w))
		}
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return rows
}

func trackSub(t library.Track) string {
	if t.Artist != "" {
		if t.Album != "" {
			return t.Artist + " - " + t.Album
		}
		return t.Artist
	}
	if t.Album != "" {
		return t.Album
	}
	return t.Path
}

// rightRows renders the side-mode song card: the square cover art
// (never boxed, never stretched) over the title/artist/album/format
// lines, which are centered UNDER THE ART — constrained to the art
// width so a long title can't spill past the cover and look
// off-center. The whole block is vertically centered. Each row is
// exactly rightW cells wide.
func (m Model) rightRows(lo layout, h int) []string {
	w := max(lo.rightW, 0)
	rows := make([]string, 0, h)
	if w <= 0 || h <= 0 {
		return rows
	}
	var content []string
	if lo.showArt {
		art := m.artText
		if art == "" {
			art = placeholderArt(lo.artW, lo.artH)
		}
		for _, a := range fitCells(art, lo.artW, lo.artH) {
			content = append(content, centerCells(a, w))
		}
		content = append(content, strings.Repeat(" ", w))
	}
	iw := w
	if lo.showArt {
		iw = max(min(lo.artW, w), 8)
	}
	title, artist, album, format := m.cardText()
	for _, ln := range []string{
		centerCells(nowStyle.Render(truncCells(title, iw)), iw),
		centerCells(dimStyle.Render(truncCells(artist, iw)), iw),
		centerCells(dimStyle.Render(truncCells(album, iw)), iw),
		centerCells(fmtStyle.Render(truncCells(format, iw)), iw),
	} {
		content = append(content, centerCells(ln, w))
	}
	for i := 0; i < max((h-len(content))/2, 0); i++ {
		rows = append(rows, strings.Repeat(" ", w))
	}
	rows = append(rows, content...)
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return rows
}

// cardRows renders the stacked-mode song card: the square cover on
// the left with the info lines beside it, both vertically centered,
// no boxes anywhere. Exactly cardH rows of W cells.
func (m Model) cardRows(lo layout) []string {
	fw := max(lo.W, 0)
	h := lo.cardH
	rows := make([]string, 0, h)
	if fw <= 0 || h <= 0 {
		return rows
	}
	title, artist, album, format := m.cardText()
	text := []string{
		nowStyle.Render(truncCells(title, fw)),
		dimStyle.Render(truncCells(artist, fw)),
		dimStyle.Render(truncCells(album, fw)),
		fmtStyle.Render(truncCells(format, fw)),
	}
	if !lo.showArt {
		// Text only, centered both ways.
		for i := 0; i < max((h-len(text))/2, 0) && len(rows) < h; i++ {
			rows = append(rows, strings.Repeat(" ", fw))
		}
		for _, t := range text {
			if len(rows) >= h {
				break
			}
			rows = append(rows, centerCells(t, fw))
		}
		for len(rows) < h {
			rows = append(rows, strings.Repeat(" ", fw))
		}
		return rows
	}
	// 1-cell left margin + art + 2-cell gap, text takes the rest.
	zoneW := 1 + lo.artW + 2
	textW := fw - zoneW
	if textW < 8 {
		// Shouldn't happen (layout reserves the text column), but
		// never break the frame: fall back to a centered text card.
		lo.showArt = false
		return m.cardRows(lo)
	}
	for i, t := range text {
		text[i] = padCells(t, textW)
	}
	art := m.artText
	if art == "" {
		art = placeholderArt(lo.artW, lo.artH)
	}
	aart := fitCells(art, lo.artW, lo.artH)
	artTopPad := max((h-len(aart))/2, 0)
	textTopPad := max((h-len(text))/2, 0)
	for i := 0; i < h; i++ {
		var l, r string
		if j := i - artTopPad; j >= 0 && j < len(aart) {
			l = " " + aart[j] + "  "
		} else {
			l = strings.Repeat(" ", zoneW)
		}
		if j := i - textTopPad; j >= 0 && j < len(text) {
			r = text[j]
		} else {
			r = strings.Repeat(" ", textW)
		}
		rows = append(rows, l+r) // exactly zoneW+textW == fw cells
	}
	return rows
}

func (m Model) cardText() (title, artist, album, format string) {
	if m.cur >= 0 && m.cur < len(m.tracks) {
		t := m.tracks[m.cur]
		title = t.Title
		artist, album = t.Artist, t.Album
		if artist == "" && album == "" {
			artist = t.Path
		}
	} else {
		title = "— nothing playing —"
		artist = "enter on a track to play"
	}
	ext, rate, ch := m.player.Info()
	format = formatLine(ext, rate, ch)
	if len(m.tracks) > 0 && m.cur >= 0 {
		format += fmt.Sprintf("   %d/%d", m.cur+1, len(m.tracks))
	}
	return title, artist, album, format
}

// controlsText is "⟳ ⮂   ⏸   0:18 ───…─── 3:27", bar fills the row.
func (m Model) controlsText(w int) string {
	if w <= 10 {
		return truncCells(m.controlsTextPlain(), w)
	}
	// Any active repeat reads bold; repeat-one adds a ₁ counter.
	repGlyph := "⟳"
	if m.cfg.Repeat == config.RepeatOne {
		repGlyph = "⟳₁"
	}
	rep := dimStyle.Render(repGlyph)
	if m.cfg.Repeat != config.RepeatOff {
		rep = pillBold.Render(repGlyph)
	}
	sh := dimStyle.Render("⮂")
	if m.cfg.Shuffle {
		sh = pillBold.Render("⮂") // on: highlighted + bold
	}
	// Icon follows the transport, not m.playing (Toggle flips playing
	// to false while paused — gating on it would draw ■ on pause).
	play := "■"
	if m.cur >= 0 {
		if m.player.Paused() {
			play = "▶"
		} else {
			play = "⏸"
		}
	}
	pos, length := m.player.Position(), m.player.Length()
	pct := 0.0
	if length > 0 {
		pct = min(max(float64(pos)/float64(length), 0), 1)
	}
	el, tot := fmtDur(pos), fmtDur(length)
	prefix := rep + " " + sh + "   " + play + "   " + el + " "
	suffix := " " + tot
	barW := max(w-cellWidth(prefix)-cellWidth(suffix), 4)
	filled := int(pct * float64(barW))
	bar := titleStyle.Render(strings.Repeat("─", filled)) + dimStyle.Render(strings.Repeat("─", barW-filled))
	return prefix + bar + suffix
}

func (m Model) controlsTextPlain() string {
	pos, length := m.player.Position(), m.player.Length()
	play := "■"
	if m.cur >= 0 {
		if m.player.Paused() {
			play = "▶"
		} else {
			play = "⏸"
		}
	}
	repGlyph := "⟳"
	if m.cfg.Repeat == config.RepeatOne {
		repGlyph = "⟳₁"
	}
	return repGlyph + " ⮂   " + play + "   " + fmtDur(pos) + " " + fmtDur(length)
}

// volumeText is "Volume: 100% (+/- to change, Shift for precise)"
// left-aligned with "press k for keybinds" right-aligned.
// Errors/status surface here so the frame stays exact.
func (m Model) volumeText(w int) string {
	hint := "press k for keybinds"
	var left string
	if m.errText != "" {
		left = "! " + m.errText
	} else if m.status != "" {
		left = "* " + m.status
	} else if m.player.Muted() {
		left = "Volume: muted (m to unmute)"
	} else {
		left = fmt.Sprintf("Volume: %d%% (+/- to change, Shift for precise)", m.player.Volume())
	}
	if cellWidth(left)+1+cellWidth(hint) > w {
		// Prefer keeping the hint: shrink the left side.
		room := max(w-cellWidth(hint)-1, 0)
		left = truncCells(left, room)
	}
	gap := max(w-cellWidth(left)-cellWidth(hint), 1)
	return left + strings.Repeat(" ", gap) + dimStyle.Render(hint)
}

// helpFrame renders the keybinds overlay as the full middle section:
// a centered dialog over blank rows — exactly midH lines so the
// frame stays exact.
// helpLines is the single source for the keybinds list, shared by
// the main overlay and the tab overlay.
var helpLines = []string{
	"space / p      play / pause",
	"← / →          previous / next track",
	"↑ / ↓, wheel   move in the list (enter plays)",
	"enter          play selected",
	"+ / -          volume ±5 (shift: ±1 precise)",
	"m              mute on/off",
	"a / d          seek -5s / +5s",
	"s              shuffle on/off",
	"r              repeat off → all → one",
	"u              refresh library",
	"f              search (title / artist / album)",
	"y              download from YouTube",
	"v              visualizer (cover-colored bars)",
	"k              keybinds",
	"c              settings",
	"esc            back",
	"q              quit",
	"*              hotkeys work in any layout",
}

func (m Model) helpFrame(lo layout) []string {
	W := lo.W
	body := helpLines
	bw := 0
	for _, l := range body {
		bw = max(bw, cellWidth(l))
	}
	bw = min(max(bw+4, 20), max(W-6, 20))
	box := []string{"┌" + strings.Repeat("─", bw-2) + "┐"}
	box = append(box, "│ "+padCells(titleStyle.Render("keybinds"), bw-3)+"│")
	for _, l := range body {
		box = append(box, "│ "+padCells(l, bw-3)+"│")
	}
	box = append(box, "└"+strings.Repeat("─", bw-2)+"┘")
	if len(box) > lo.midH {
		box = box[:max(lo.midH, 0)] // tiny window: trim, never overflow
	}
	pad := max((W-bw)/2, 0)
	out := []string{}
	blankTop := max((lo.midH-len(box))/2, 0)
	blankBot := max(lo.midH-len(box)-blankTop, 0)
	for i := 0; i < blankTop; i++ {
		out = append(out, strings.Repeat(" ", W))
	}
	for _, b := range box {
		out = append(out, strings.Repeat(" ", pad)+b+strings.Repeat(" ", max(W-pad-cellWidth(b), 0)))
	}
	for i := 0; i < blankBot; i++ {
		out = append(out, strings.Repeat(" ", W))
	}
	return out
}

// cell helpers (visible-width aware, ANSI-safe via lipgloss.Width).
func cellWidth(s string) int { return lipgloss.Width(s) }

func padCells(s string, w int) string {
	if w <= 0 {
		return ""
	}
	wd := lipgloss.Width(s)
	if wd > w {
		return truncCells(s, w)
	}
	if wd == w {
		return s
	}
	return s + strings.Repeat(" ", w-wd)
}

func centerCells(s string, w int) string {
	if w <= 0 {
		return ""
	}
	wd := lipgloss.Width(s)
	if wd > w {
		return truncCells(s, w)
	}
	if wd == w {
		return s
	}
	left := (w - wd) / 2
	right := w - wd - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// truncCells shortens s to n cells, appending … when shortened. It is
// ANSI-escape aware (escape sequences pass through uncounted) so
// styled text can never corrupt the frame by miscounting styles as
// cells — the old rune-based cut turned exactly-fitting styled lines
// (progress bar, volume hint) into "…" garbage.
func truncCells(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	var b strings.Builder
	w := 0
	inEsc := false
	sawEsc := false
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			sawEsc = true
			b.WriteRune(r)
			continue
		}
		rw := lipgloss.Width(string(r))
		if w+rw > n-1 { // reserve one cell for …
			break
		}
		b.WriteRune(r)
		w += rw
	}
	if sawEsc {
		// The cut may have dropped the terminating reset (e.g. a
		// resized cover-art line): close it or the background
		// leaks onto padding and neighbouring text.
		b.WriteString("\x1b[0m")
	}
	b.WriteString("…")
	return b.String()
}

// fitCells forces art (lines with ANSI escapes) to exactly w×h cells.
func fitCells(art string, w, h int) []string {
	lines := strings.Split(art, "\n")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		var ln string
		if i < len(lines) {
			ln = lines[i]
		}
		wd := lipgloss.Width(ln)
		if wd < w {
			ln += strings.Repeat(" ", w-wd)
		} else if wd > w {
			ln = truncCells(ln, w)
		}
		out = append(out, ln)
	}
	return out
}

// viewHeader is superseded by headerText (kept for compatibility).

// viewCard renders the song card: cover art + song name + format.
// viewCard is superseded by rightRows (kept for compatibility).

// viewControls renders the full-width bottom bar: progress + time,
// then state, volume, toggles and queue position.
// viewControls is superseded by controlsText (kept for compatibility).

// subHeader is the shared frame header for the settings/download
// tabs: same brand, same 1-cell indent as the main player view.
func subHeader(section string) string {
	return brandStyle.Render("GoMusic") + dimStyle.Render("   "+section)
}

// subView renders the settings/download tabs in the main player
// frame style: borderless, 1-cell indent, header + blank, body,
// spacer, the same volume/hint footer — exactly H lines, with no
// box-drawing glyphs anywhere.
func (m Model) subView(section string, body []string) string {
	W, H := m.width, m.height
	if W < 10 || H < 6 {
		return padCells(truncCells(subHeader(section), max(W, 1)), max(W, 1))
	}
	lines := make([]string, 0, H)
	lines = append(lines, " "+padCells(subHeader(section), W-1))
	lines = append(lines, strings.Repeat(" ", W))
	room := max(H-4, 1)
	for i := 0; i < room; i++ {
		ln := ""
		if i < len(body) {
			ln = body[i]
		}
		lines = append(lines, " "+padCells(ln, W-1))
	}
	lines = append(lines, strings.Repeat(" ", W))
	lines = append(lines, " "+padCells(m.volumeText(W-1), W-1))
	for len(lines) < H {
		lines = append(lines, strings.Repeat(" ", W))
	}
	return strings.Join(lines[:min(len(lines), H)], "\n")
}

// subHelpBody shows the keybinds overlay inside the tab frame when
// "k" is pressed there — same list as the main overlay, no box.
func (m Model) subHelpBody() []string {
	body := []string{titleStyle.Render("keybinds"), ""}
	return append(body, helpLines...)
}

func (m Model) viewSettings() string {
	if m.showHelp {
		return m.subView("Settings", m.subHelpBody())
	}
	if m.browsing {
		return m.subView("Settings", m.folderBrowserBody())
	}
	d := m.cfg.Discord
	onOffBadge := func(b bool) string {
		if b {
			return badgeOn.Render("ON")
		}
		return badgeOff.Render("OFF")
	}
	mark := func(row settingRow) string {
		if m.currentRow() == row {
			return titleStyle.Render("▸ ")
		}
		return "  "
	}
	folderVal := dimStyle.Render(truncCells(m.cfg.MusicDir, max(m.width-18, 1)))
	if m.editingPath {
		folderVal = m.pathInput.View()
	}
	body := []string{
		mark(rowFolder) + "Music folder: " + folderVal,
		mark(rowDiscord) + "Discord Rich Presence: " + onOffBadge(d.Enabled),
	}
	if d.Enabled {
		body = append(body, mark(rowShowButton)+"GitHub button: "+onOffBadge(d.ShowButton))
	}
	body = append(body, "")
	body = append(body, "  "+dimStyle.Render("Status: ")+m.discordStatusLine())
	if m.editingPath {
		body = append(body, dimStyle.Render("paste/type a path • enter use it • esc cancel"))
	} else {
		body = append(body, dimStyle.Render("↑/↓ or wheel choose • enter change • l browse folders • esc back"))
	}
	return m.subView("Settings", body)
}

// folderBrowserBody is the folder-pick sub-mode: heading + path +
// directories-only rows filling the frame + hints.
func (m Model) folderBrowserBody() []string {
	body := []string{
		sectionStyle.Render("Music folder:"),
		"  " + dimStyle.Render(truncCells(m.dirPath, max(m.width-4, 1))),
	}
	bh := m.browserH()
	if m.dirErr != "" {
		body = append(body, "  "+errStyle.Render("! "+truncCells(m.dirErr, max(m.width-5, 1))))
		for i := 1; i < bh; i++ {
			body = append(body, "")
		}
	} else if len(m.dirNames) == 0 {
		body = append(body, "  "+dimStyle.Render("…"))
		for i := 1; i < bh; i++ {
			body = append(body, "")
		}
	} else {
		for i := 0; i < bh; i++ {
			idx := m.dirOffset + i
			if idx >= len(m.dirNames) {
				body = append(body, "")
				continue
			}
			label := dirLabel(m.dirNames[idx])
			if idx == m.dirCursor {
				body = append(body, "  "+titleStyle.Render("▸ "+truncCells(label, max(m.width-7, 1))))
			} else {
				body = append(body, "    "+truncCells(label, max(m.width-6, 1)))
			}
		}
	}
	body = append(body, dimStyle.Render("↑/↓ move • enter open • a use folder • esc back"))
	return body
}

// Download (YouTube) ------------------------------------------------

// dlCheckMsg carries the link-check result; dlDoneMsg the download result.
// gen drops stale results when the user cancels or restarts.
type dlCheckMsg struct {
	gen   int
	link  string
	title string
	count int
	err   string
}

type dlDoneMsg struct {
	gen   int
	files []string
	err   string
}

func (m Model) updateDownload(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			if m.dlBusy && m.dlCancel != nil {
				m.dlCancel()
				m.dlGen++ // in-flight results become stale
				m.dlBusy = false
				m.dlProg = nil
				m.dlStatus = "cancelled"
			}
			m.mode = modeMain
			m.urlInput.Blur()
			return m, nil
		case msg.String() == "enter":
			return m.startDownload()
		}
	case dlCheckMsg:
		if msg.gen != m.dlGen {
			return m, nil // stale (cancelled/superseded)
		}
		if msg.err != "" {
			m.dlBusy = false
			m.dlStatus = msg.err
			return m, nil
		}
		if msg.count > 1 {
			m.dlStatus = fmt.Sprintf("found %d tracks (%s…) — downloading…", msg.count, truncCells(msg.title, 40))
		} else {
			m.dlStatus = fmt.Sprintf("found %s — downloading…", truncCells(msg.title, 60))
		}
		return m, m.dlFetchCmd(msg.gen, msg.link, msg.count)
	case dlDoneMsg:
		if msg.gen != m.dlGen {
			return m, nil // stale
		}
		m.dlBusy = false
		m.dlProg = nil
		if msg.err != "" {
			m.dlStatus = msg.err
			return m, nil
		}
		names := make([]string, 0, len(msg.files))
		for _, f := range msg.files {
			names = append(names, strings.TrimSuffix(filepath.Base(f), ".mp3"))
		}
		shown := strings.Join(names, ", ")
		if len(msg.files) > 3 {
			shown = strings.Join(names[:3], ", ") + fmt.Sprintf(", +%d more", len(msg.files)-3)
		}
		m.dlStatus = fmt.Sprintf("saved %d file(s) to %s: %s", len(msg.files), m.cfg.MusicDir, shown)
		return m, m.loadLibrary(m.cfg.MusicDir)
	}
	var cmd tea.Cmd
	m.urlInput, cmd = m.urlInput.Update(msg)
	return m, cmd
}

// startDownload validates the link and tools, then kicks off check → fetch.
func (m Model) startDownload() (tea.Model, tea.Cmd) {
	if m.dlBusy {
		return m, nil
	}
	link, err := downloader.ValidURL(m.urlInput.Value())
	if err != nil {
		m.dlStatus = err.Error()
		return m, nil
	}
	ytBin, err := downloader.Bins()
	if err != nil {
		m.dlStatus = err.Error()
		return m, nil
	}
	m.dlBusy = true
	m.dlGen++
	m.dlProg = nil
	m.dlStatus = "checking link…"
	ctx, cancel := context.WithCancel(context.Background())
	m.dlCancel = cancel
	gen := m.dlGen
	return m, func() tea.Msg {
		title, n, err := downloader.Check(ctx, ytBin, link)
		if err != nil {
			return dlCheckMsg{gen: gen, err: err.Error()}
		}
		return dlCheckMsg{gen: gen, link: link, title: title, count: n}
	}
}

// dlFetchCmd starts the fetch phase. Pointer receiver on purpose:
// the fetch context's cancel must land on the model the caller
// returns, or esc during a download would cancel nothing (the old
// value-receiver version dropped it on the floor).
func (m *Model) dlFetchCmd(gen int, link string, total int) tea.Cmd {
	ytBin, _ := downloader.Bins()
	dir := m.cfg.MusicDir
	prog := downloader.NewProgress(total)
	m.dlProg = prog
	ctx, cancel := context.WithCancel(context.Background())
	m.dlCancel = cancel
	return func() tea.Msg {
		files, err := downloader.DownloadWithProgress(ctx, ytBin, link, dir, prog)
		if err != nil {
			return dlDoneMsg{gen: gen, err: err.Error()}
		}
		return dlDoneMsg{gen: gen, files: files}
	}
}

func (m Model) viewDownload() string {
	if m.showHelp {
		return m.subView("Download", m.subHelpBody())
	}
	var body []string
	body = append(body, sectionStyle.Render("YouTube / YouTube Music link:"))
	body = append(body, "  "+m.urlInput.View())
	body = append(body, "")
	if m.dlBusy {
		body = append(body, m.dlProgressLines()...)
	} else if m.dlStatus != "" {
		body = append(body, truncCells(m.dlStatus, max(m.width-2, 1)))
	}
	body = append(body, "")
	body = append(body, dimStyle.Render("enter downloads mp3 into your music folder • playlists download whole • esc backs out"))
	return m.subView("Download", body)
}

// dlProgressLines renders the running download in the player style:
// track position + title for playlists, one shared bar. The view
// refreshes on the 500ms tick while yt-dlp streams into dlProg.
func (m Model) dlProgressLines() []string {
	total, done, cur, pct := m.dlProg.Snapshot()
	if m.dlProg == nil || (total <= 1 && cur == "") {
		return []string{titleStyle.Render("… " + m.dlStatus)}
	}
	var out []string
	if total > 1 {
		pos := fmt.Sprintf("Track %d of %d", min(done+1, total), total)
		out = append(out, titleStyle.Render(pos)+dimStyle.Render("  "+truncCells(cur, max(m.width-len(pos)-6, 1))))
	} else {
		out = append(out, titleStyle.Render("… "+truncCells(cur, max(m.width-6, 1))))
	}
	overall := m.dlProg.Overall()
	barW := max(m.width-12, 10)
	filled := int(overall * float64(barW))
	bar := titleStyle.Render(strings.Repeat("─", filled)) + dimStyle.Render(strings.Repeat("─", barW-filled))
	out = append(out, "  "+bar+fmt.Sprintf(" %3.0f%%", overall*100))
	if pct >= 100 {
		out = append(out, dimStyle.Render("  converting / tagging…"))
	}
	return out
}
