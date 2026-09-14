package ui

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

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
type discordRefreshMsg struct{}
type trackEndedMsg struct{ gen int }
type libraryMsg struct{ tracks []library.Track }
type errMsg struct{ err error }
type statusClearMsg struct{}
type artMsg struct {
	path string
	w, h int
	art  string
}

// Key bindings ----------------------------------------------------

type keyMap struct {
	Play     key.Binding
	Pause    key.Binding
	Next     key.Binding
	Prev     key.Binding
	Shuffle  key.Binding
	Repeat   key.Binding
	VolUp    key.Binding
	VolDown  key.Binding
	Mute     key.Binding
	SeekFwd  key.Binding
	SeekBack key.Binding
	Settings key.Binding
	ApplyDir key.Binding
	Download key.Binding
	Back     key.Binding
	Help     key.Binding
	Quit     key.Binding
	Stop     key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Play, k.Pause, k.Next, k.Prev, k.Mute, k.Help, k.Settings, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Play, k.Pause, k.Stop, k.Next, k.Prev},
		{k.Shuffle, k.Repeat, k.VolUp, k.VolDown, k.Mute},
		{k.SeekFwd, k.SeekBack, k.Settings, k.Download, k.Help, k.Back, k.Quit},
	}
}

var keys = keyMap{
	Play:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "play")),
	Pause:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "pause")),
	Stop:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
	Next:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
	Prev:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev")),
	Shuffle:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shuffle")),
	Repeat:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeat")),
	VolUp:    key.NewBinding(key.WithKeys("+", "=", "up"), key.WithHelp("+", "vol+")),
	VolDown:  key.NewBinding(key.WithKeys("-", "_", "down"), key.WithHelp("-", "vol-")),
	Mute:     key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mute")),
	SeekFwd:  key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("→", "+5s")),
	SeekBack: key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("←", "-5s")),
	Settings: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "settings")),
	Download: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "download")),
	ApplyDir: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "use folder")),
	Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:     key.NewBinding(key.WithKeys("k", "?"), key.WithHelp("k", "keybinds")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
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
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2)
	sideStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	fmtStyle     = lipgloss.NewStyle().Foreground(accent2)
	queueStyle   = lipgloss.NewStyle().Foreground(dimColor).Bold(true)
	sectionStyle = lipgloss.NewStyle().Foreground(dimColor).Bold(true)
	// Active-state pills: an accent background so the state reads at a
	// glance. pillBold adds bold (playing track, shuffle ON, repeat
	// ONE); pillSoft stays regular weight (repeat ALL).
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
	prog     progress.Model
	picker   filepicker.Model
	help     help.Model
	mode     mode
	width    int
	height   int
	cur      int // index into tracks, -1 = none
	playing  bool
	playGen  int // increments on every play/stop; drops stale trackEndedMsg
	showHelp bool
	status   string
	errText  string
	// Cover art cache (rendered cells for the current box size).
	artPath string
	artText string
	artW    int
	artH    int
	// Discord Rich Presence (optional; all IPC off the main path).
	discord *discord.Manager
	// Ticks since start (presence re-pushes every ~30s).
	ticks int
	// Settings focus: folder picker or one of the Discord inputs.
	setFocus      int
	clientIDInput textinput.Model
	// YouTube download tab.
	urlInput textinput.Model
	dlStatus string
	dlBusy   bool
	dlGen    int
	dlCancel context.CancelFunc
}

// Settings focus zones.
const (
	focusPicker = iota
	focusClientID
)

func New(cfg *config.Config, p *player.Player) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(nil, delegate, 60, 15)
	l.Title = "Library"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	// "k" opens keybinds (see footer) so the list must not steal it:
	// cursor moves on ↑/↓ only.
	l.KeyMap.CursorUp.SetKeys("up")
	l.KeyMap.CursorDown.SetKeys("down")
	l.KeyMap.PrevPage.SetKeys("pgup")
	l.KeyMap.NextPage.SetKeys("pgdown")

	prog := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())

	fp := filepicker.New()
	fp.DirAllowed = false // selection = current dir via "a", keeps enter purely for navigation
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.CurrentDirectory = cfg.MusicDir
	fp.Height = 12

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "Discord Application ID"
	ci.CharLimit = 32
	ci.SetValue(cfg.Discord.ClientID)

	ui := textinput.New()
	ui.Prompt = ""
	ui.Placeholder = "https://music.youtube.com/… or youtube.com/…"
	ui.CharLimit = 2048

	dm := discord.New()
	dm.Configure(discordSettingsOf(cfg))

	return Model{
		cfg: cfg, player: p, list: l, prog: prog, picker: fp,
		help: help.New(), mode: modeMain, cur: -1,
		discord: dm, clientIDInput: ci, urlInput: ui,
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
		m.picker.Init(),
		tick(),
		m.syncDiscordCmd(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func discordRefreshAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return discordRefreshMsg{} })
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

// loadArtCmd decodes + renders cover art off the main path.
func loadArtCmd(path string, w, h int) tea.Cmd {
	return func() tea.Msg {
		img, err := library.Cover(path)
		if err != nil {
			return artMsg{path: path, w: w, h: h, art: ""}
		}
		return artMsg{path: path, w: w, h: h, art: renderCover(img, w, h)}
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
	m.picker.Height = max(m.height-10, 4)
	m.help.Width = m.width
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
	m.cur = i
	m.playing = true
	m.list.Select(i)
	m.errText = ""
	lo := m.layout()
	if lo.showArt {
		m.artW, m.artH = lo.artW, lo.artH
		cmds = append(cmds, loadArtCmd(m.tracks[i].Path, lo.artW, lo.artH))
	}
	cmds = append(cmds, m.waitTrackEnd(gen), m.pushPresenceCmd())
	return tea.Batch(cmds...)
}

func (m *Model) stopPlayback() tea.Cmd {
	cmds := m.markPlaying(m.cur, -1)
	m.playGen++ // invalidate pending trackEndedMsg
	m.player.Stop()
	m.playing = false
	m.cur = -1
	m.artPath, m.artText = "", ""
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) nextIndex(step int) int {
	n := len(m.tracks)
	if n == 0 {
		return -1
	}
	if m.cfg.Shuffle && n > 1 {
		for {
			j := rand.Intn(n)
			if j != m.cur {
				return j
			}
		}
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

// trunc shortens s to n runes with an ellipsis.
func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		m.tracks = msg.tracks
		m.setItems()
		if len(m.tracks) == 0 {
			m.status = "no music found — press S to choose a folder"
		} else {
			m.status = fmt.Sprintf("%d tracks", len(m.tracks))
		}
		return m, clearStatusAfter(3 * time.Second)

	case errMsg:
		m.errText = msg.err.Error()
		return m, nil

	case statusClearMsg:
		m.status = ""
		return m, nil

	case tickMsg:
		m.ticks++
		if m.ticks%60 == 0 {
			// Re-detect Discord started after us; refresh timestamps.
			return m, tea.Batch(tick(), m.pushPresenceCmd())
		}
		return m, tick()

	case discordRefreshMsg:
		return m, nil

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
		if msg.art == "" {
			m.artText = placeholderArt(msg.w, msg.h)
		} else {
			m.artText = msg.art
		}
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
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.persist()
			m.discord.Shutdown()
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, keys.Download):
			m.mode = modeDownload
			m.urlInput.Focus()
			if _, _, err := downloader.Bins(); err != nil {
				m.dlStatus = err.Error()
			} else if m.dlStatus == "" {
				m.dlStatus = "paste a YouTube / YouTube Music link — playlists download whole"
			}
			return m, textinput.Blink
		case key.Matches(msg, keys.Settings):
			m.mode = modeSettings
			m.picker.CurrentDirectory = m.cfg.MusicDir
			m.setFocus = focusPicker
			m.blurInputs()
			return m, m.picker.Init()
		case key.Matches(msg, keys.Pause):
			if m.cur >= 0 {
				m.player.Toggle()
				m.playing = !m.player.Paused()
			}
			return m, m.pushPresenceCmd()
		case key.Matches(msg, keys.Play):
			if sel, ok := m.list.SelectedItem().(trackItem); ok {
				for i, t := range m.tracks {
					if t.Path == sel.t.Path {
						return m, m.playIndex(i)
					}
				}
			}
			return m, nil
		case key.Matches(msg, keys.Stop):
			stopCmd := m.stopPlayback()
			return m, tea.Batch(stopCmd, m.pushPresenceCmd())
		case key.Matches(msg, keys.Next):
			j := m.nextIndex(1)
			if j >= 0 {
				return m, m.playIndex(j)
			}
			m.status = "end of queue (repeat is off)"
			return m, clearStatusAfter(2 * time.Second)
		case key.Matches(msg, keys.Prev):
			if m.player.Position() > 3*time.Second {
				m.player.SeekBy(-m.player.Position())
				return m, m.pushPresenceCmd()
			}
			j := m.nextIndex(-1)
			if j >= 0 {
				return m, m.playIndex(j)
			}
			return m, nil
		case key.Matches(msg, keys.Shuffle):
			m.cfg.Shuffle = !m.cfg.Shuffle
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
			m.player.ToggleMute()
			return m, nil
		case key.Matches(msg, keys.SeekFwd):
			m.player.SeekBy(5 * time.Second)
			return m, m.pushPresenceCmd()
		case key.Matches(msg, keys.SeekBack):
			m.player.SeekBy(-5 * time.Second)
			return m, m.pushPresenceCmd()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(km, keys.Quit) {
			m.saveDiscordInputs()
			m.persist()
			m.discord.Shutdown()
			return m, tea.Quit
		}
		if m.setFocus != focusPicker {
			// Typing in a Discord input: esc/enter/tab blurs,
			// everything else goes to the input.
			if key.Matches(km, keys.Back) || km.Type == tea.KeyEnter || km.String() == "tab" {
				m.saveDiscordInputs()
				m.persist()
				if km.String() == "tab" {
					m.cycleFocus()
				} else {
					m.setFocus = focusPicker
					m.blurInputs()
				}
				return m, m.syncDiscordCmd()
			}
			return m.updateDiscordInput(msg)
		}
		switch {
		case key.Matches(km, keys.Back):
			m.mode = modeMain
			return m, m.pushPresenceCmd()
		case key.Matches(km, keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(km, keys.ApplyDir):
			dir := m.picker.CurrentDirectory
			m.cfg.MusicDir = dir
			m.persist()
			m.mode = modeMain
			stopCmd := m.stopPlayback()
			m.status = "library: " + dir
			return m, tea.Batch(stopCmd, m.loadLibrary(dir), clearStatusAfter(3*time.Second))
		case km.String() == "tab":
			m.setFocus = focusClientID
			m.focusInputs()
			return m, textinput.Blink
		case km.String() == "d":
			m.cfg.Discord.Enabled = !m.cfg.Discord.Enabled
			m.persist()
			m.status = "discord: " + onOff(m.cfg.Discord.Enabled)
			return m, tea.Batch(m.syncDiscordCmd(), clearStatusAfter(2*time.Second), discordRefreshAfter(2*time.Second))
		case km.String() == "t":
			m.cfg.Discord.ShowTime = !m.cfg.Discord.ShowTime
			m.persist()
			return m, m.syncDiscordCmd()
		case km.String() == "b":
			m.cfg.Discord.ShowButton = !m.cfg.Discord.ShowButton
			m.persist()
			return m, m.syncDiscordCmd()
		}
	}
	if m.setFocus != focusPicker {
		return m.updateDiscordInput(msg)
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

// updateDiscordInput routes messages to the focused settings input.
func (m Model) updateDiscordInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.clientIDInput, cmd = m.clientIDInput.Update(msg)
	return m, cmd
}

func (m *Model) cycleFocus() {
	if m.setFocus == focusClientID {
		m.setFocus = focusPicker
	} else {
		m.setFocus = focusClientID
	}
	m.focusInputs()
}

func (m *Model) focusInputs() {
	m.clientIDInput.Blur()
	if m.setFocus == focusClientID {
		m.clientIDInput.Focus()
	}
}

func (m *Model) blurInputs() {
	m.clientIDInput.Blur()
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

func (m *Model) saveDiscordInputs() {
	m.cfg.Discord.ClientID = strings.TrimSpace(m.clientIDInput.Value())
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
	full := "GoMusic   " + lib + "   " + m.cfg.MusicDir
	if cellWidth(full) <= inner {
		return brandStyle.Render("GoMusic") + "   " + lib + "   " + dimStyle.Render(m.cfg.MusicDir)
	}
	// Shrink the directory first, never the brand/counts.
	keep := "GoMusic   " + lib + "   "
	room := max(inner-cellWidth(keep), 0)
	return brandStyle.Render("GoMusic") + "   " + lib + "   " + dimStyle.Render(truncCells(m.cfg.MusicDir, room))
}

// leftRows renders the track list with a 1-cell leading indent. The
// playing track uses "» " on the title and aligns the subtitle
// under it with 3 leading spaces. Exactly h rows (blank-padded).
func (m Model) leftRows(w, h, rowsPer int, showAll bool) []string {
	rows := make([]string, 0, h)
	if w <= 0 || h <= 0 {
		return rows
	}
	// Filter input replaces the first row while typing.
	if m.list.FilterState() == list.Filtering {
		q := "/" + m.list.FilterValue() + "▌"
		rows = append(rows, " "+padCells(truncCells(q, w-1), w-1))
		for len(rows) < h {
			rows = append(rows, strings.Repeat(" ", w))
		}
		return rows
	}
	items := m.list.VisibleItems()
	sel := m.list.Index()
	n := len(items)
	if n == 0 {
		msg := " no music — press S to choose a folder"
		if m.list.IsFiltered() {
			msg = " no matches"
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

// controlsText is "⟳ ⇄   ⏸   0:18 ───…─── 3:27", bar fills the row.
func (m Model) controlsText(w int) string {
	if w <= 10 {
		return truncCells(m.controlsTextPlain(), w)
	}
	rep := dimStyle.Render("⟳")
	switch m.cfg.Repeat {
	case config.RepeatOne:
		rep = pillBold.Render("⟳") // one song: highlighted + bold
	case config.RepeatAll:
		rep = pillSoft.Render("⟳") // all: highlighted, regular weight
	}
	sh := dimStyle.Render("⇄")
	if m.cfg.Shuffle {
		sh = pillBold.Render("⇄") // on: highlighted + bold
	}
	play := "■"
	if m.cur >= 0 && m.playing {
		if m.player.Paused() {
			play = "▶"
		} else {
			play = "⏸"
		}
	}
	pos, length := m.player.Position(), m.player.Length()
	pct := 0.0
	if length > 0 {
		pct = clampFloat(float64(pos)/float64(length), 0, 1)
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
	if m.cur >= 0 && m.playing {
		if m.player.Paused() {
			play = "▶"
		} else {
			play = "⏸"
		}
	}
	return "⟳ ⇄   " + play + "   " + fmtDur(pos) + " " + fmtDur(length)
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
		left = fmt.Sprintf("Volume: muted (+/- to unmute, Shift for precise)")
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
func (m Model) helpFrame(lo layout) []string {
	W := lo.W
	body := []string{
		"enter        play selected",
		"space        pause / resume",
		"x            stop",
		"n / p        next / previous (p restarts if >3s in)",
		"s            shuffle on/off",
		"r            repeat off → all → one",
		"+ / -        volume ±5  (shift: + / _ = ±1 precise)",
		"m            mute",
		"← / →        seek ∓5s  (h / l works too)",
		"/            filter library",
		"S            settings (music folder)",
		"y            download from YouTube",
		"esc          back",
		"q            quit",
	}
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
		rw := runewidth.RuneWidth(r)
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

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m Model) viewSettings() string {
	m.picker.Height = max(m.height-17, 3)
	m.help.Width = m.width
	m.help.ShowAll = m.showHelp
	d := m.cfg.Discord
	discordState := badgeOff.Render("OFF")
	if d.Enabled {
		discordState = badgeOn.Render("ON")
	}
	timeState, btnState := badgeOff.Render("OFF"), badgeOff.Render("OFF")
	if d.ShowTime {
		timeState = badgeOn.Render("ON")
	}
	if d.ShowButton {
		btnState = badgeOn.Render("ON")
	}
	focusMark := func(f int) string {
		if m.setFocus == f {
			return titleStyle.Render("▸ ")
		}
		return "  "
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ settings"))
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Music folder: ") + trunc(m.picker.CurrentDirectory, max(m.width-16, 10)) + "\n")
	b.WriteString(m.picker.View())
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Discord Rich Presence: ") + discordState + dimStyle.Render("  (d to toggle)") + "\n")
	b.WriteString(focusMark(focusClientID) + dimStyle.Render("Client ID: ") + m.clientIDInput.View() + "\n")
	b.WriteString("  " + dimStyle.Render("Show time: ") + timeState + dimStyle.Render("  (t)   ") +
		dimStyle.Render("GitHub button: ") + btnState + dimStyle.Render("  (b)") + "\n")
	b.WriteString("  " + dimStyle.Render("Status: ") + m.discordStatusLine() + "\n")
	b.WriteString(boxStyle.Render(
		"Folder: ↑/↓ or j/k • open: enter/l • up: h/backspace • a uses the folder\n" +
			"Discord: tab edits ID • d/t/b toggle • esc back",
	))
	if m.status != "" {
		b.WriteString("\n" + dimStyle.Render("* "+trunc(m.status, max(m.width-4, 10))))
	}
	b.WriteString("\n" + dimStyle.Render(m.help.View(keys)))
	return b.String()
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
			m.dlStatus = fmt.Sprintf("found %d tracks (%s…) — downloading…", msg.count, trunc(msg.title, 40))
		} else {
			m.dlStatus = fmt.Sprintf("found %s — downloading…", trunc(msg.title, 60))
		}
		return m, m.dlFetchCmd(msg.gen, msg.link)
	case dlDoneMsg:
		if msg.gen != m.dlGen {
			return m, nil // stale
		}
		m.dlBusy = false
		if msg.err != "" {
			m.dlStatus = msg.err
			return m, nil
		}
		names := make([]string, 0, len(msg.files))
		for _, f := range msg.files {
			if i := strings.LastIndex(f, "/"); i >= 0 {
				f = f[i+1:]
			}
			names = append(names, strings.TrimSuffix(f, ".mp3"))
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
	ytBin, _, err := downloader.Bins()
	if err != nil {
		m.dlStatus = err.Error()
		return m, nil
	}
	m.dlBusy = true
	m.dlGen++
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

func (m Model) dlFetchCmd(gen int, link string) tea.Cmd {
	ytBin, _, _ := downloader.Bins()
	dir := m.cfg.MusicDir
	ctx, cancel := context.WithCancel(context.Background())
	m.dlCancel = cancel
	return func() tea.Msg {
		files, err := downloader.Download(ctx, ytBin, link, dir)
		if err != nil {
			return dlDoneMsg{gen: gen, err: err.Error()}
		}
		return dlDoneMsg{gen: gen, files: files}
	}
}

func (m Model) viewDownload() string {
	var b strings.Builder
	b.WriteString(brandStyle.Render("♪ GoMusic") + dimStyle.Render("  download from YouTube") + "\n\n")
	b.WriteString("Link:\n" + m.urlInput.View() + "\n\n")
	if m.dlBusy {
		b.WriteString(titleStyle.Render("… "+m.dlStatus) + "\n")
	} else if m.dlStatus != "" {
		b.WriteString(m.dlStatus + "\n")
	}
	b.WriteString("\n" + boxStyle.Render(
		"enter checks the link and downloads mp3 into your music folder\n"+
			"playlists/albums download whole • esc backs out (cancels a running download)",
	))
	b.WriteString("\n" + dimStyle.Render(m.help.View(keys)))
	return b.String()
}
