package ui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/earendil/gomusic/internal/config"
	"github.com/earendil/gomusic/internal/library"
	"github.com/earendil/gomusic/internal/player"
)

// Modes
type mode int

const (
	modeMain mode = iota
	modeSettings
)

// Messages
type tickMsg time.Time
type trackEndedMsg struct{}
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
		{k.SeekFwd, k.SeekBack, k.Settings, k.Help, k.Back, k.Quit},
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
	ApplyDir: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "use folder")),
	Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
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
	showHelp bool
	status   string
	errText  string
	// Cover art cache (rendered cells for the current box size).
	artPath string
	artText string
	artW    int
	artH    int
}

func New(cfg *config.Config, p *player.Player) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(nil, delegate, 60, 15)
	l.Title = "Library"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	prog := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())

	fp := filepicker.New()
	fp.DirAllowed = false // selection = current dir via "a", keeps enter purely for navigation
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.CurrentDirectory = cfg.MusicDir
	fp.Height = 12

	return Model{
		cfg: cfg, player: p, list: l, prog: prog, picker: fp,
		help: help.New(), mode: modeMain, cur: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadLibrary(m.cfg.MusicDir),
		m.picker.Init(),
		tick(),
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

func (m Model) waitTrackEnd() tea.Cmd {
	done := m.player.Done()
	return func() tea.Msg {
		<-done
		return trackEndedMsg{}
	}
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return statusClearMsg{} })
}

// Layout ----------------------------------------------------------

// layout is the single source of truth for adaptive sizing. Wide
// terminals get list + sidebar; narrow ones stack list over a compact
// panel; tiny ones drop the art to keep controls usable.
type layout struct {
	wide    bool
	listW   int
	listH   int
	rightW  int // wide mode: right card column width
	artW    int
	artH    int
	showArt bool
	progW   int
	volW    int
	helpH   int
	statusH int
}

const (
	headerLines    = 1
	controlsLines  = 2 // progress line + buttons line
	cardTextLines  = 3 // song title, subtitle, format
	listMinStacked = 7 // reserve list chrome + one full item (list View never clips)
)

func (m Model) layout() layout {
	W, H := m.width, m.height
	if W <= 0 || H <= 0 {
		return layout{}
	}
	var lo layout
	if m.errText != "" || m.status != "" {
		lo.statusH = 1
	}
	if m.showHelp {
		lo.helpH = len(keys.FullHelp())
	} else {
		lo.helpH = 1
	}
	midH := H - headerLines - controlsLines - lo.statusH - lo.helpH

	lo.progW = max(W-22, 8) // room for " 0:00 / 0:00" + margins
	lo.volW = clamp(W/14, 6, 12)

	lo.wide = W >= 80 && midH >= 10
	if lo.wide {
		lo.rightW = clamp(W*35/100, 26, 64)
		lo.listW = max(W-lo.rightW-1, 20)
		lo.listH = max(midH, 4)
		// Square-ish art that grows with the window.
		maxW := lo.rightW - 4
		maxH := midH - cardTextLines
		lo.artH = max(min(maxH, maxW/2), 0)
		lo.artW = lo.artH * 2
		lo.showArt = lo.artH >= 4 && lo.artW >= 8
		if !lo.showArt {
			lo.artW, lo.artH = 0, 0
		}
		return lo
	}

	// Narrow / tiny: library on top, song card below, controls last.
	// Art budget is whatever is left after protecting the list.
	maxArtH := midH - cardTextLines - listMinStacked
	lo.artH = clamp(min(14, maxArtH), 0, 14)
	lo.artW = min(lo.artH*2, max(W-4, 0))
	lo.showArt = lo.artH >= 4 && lo.artW >= 8 && W >= 36
	if !lo.showArt {
		lo.artW, lo.artH = 0, 0
	}
	lo.listW = max(W, 10)
	lo.listH = max(midH-cardTextLines-lo.artH, 3)
	return lo
}

// relayout recomputes sizes after a resize and reloads art when the
// art box changed. Returns a Cmd (possibly nil).
func (m *Model) relayout() tea.Cmd {
	lo := m.layout()
	m.picker.Height = max(m.height-10, 4)
	m.help.Width = m.width
	if m.cur < 0 || !lo.showArt {
		m.artW, m.artH = lo.artW, lo.artH
		return nil
	}
	if lo.artW == m.artW && lo.artH == m.artH && m.artPath == m.tracks[m.cur].Path {
		return nil
	}
	m.artW, m.artH = lo.artW, lo.artH
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
	cmds = append(cmds, m.waitTrackEnd())
	return tea.Batch(cmds...)
}

func (m *Model) stopPlayback() {
	m.markPlaying(m.cur, -1)
	m.player.Stop()
	m.playing = false
	m.cur = -1
	m.artPath, m.artText = "", ""
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

func (m *Model) onTrackEnded() tea.Cmd {
	switch m.cfg.Repeat {
	case config.RepeatOne:
		return m.playIndex(m.cur)
	default:
		j := m.nextIndex(1)
		if j < 0 {
			m.player.Stop() // clear length/format/position state
			m.playing = false
			m.markPlaying(m.cur, -1)
			m.cur = -1
			m.artPath, m.artText = "", ""
			m.status = "queue finished"
			return clearStatusAfter(2 * time.Second)
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
		return m, tick()

	case trackEndedMsg:
		if m.playing && m.mode == modeMain {
			return m, m.onTrackEnded()
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
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, keys.Settings):
			m.mode = modeSettings
			m.picker.CurrentDirectory = m.cfg.MusicDir
			return m, m.picker.Init()
		case key.Matches(msg, keys.Pause):
			if m.cur >= 0 {
				m.player.Toggle()
				m.playing = !m.player.Paused()
			}
			return m, nil
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
			m.stopPlayback()
			return m, nil
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
				return m, nil
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
			m.player.SetVolume(m.player.Volume() + 5)
			m.cfg.Volume = m.player.Volume()
			m.persist()
			return m, nil
		case key.Matches(msg, keys.VolDown):
			m.player.SetVolume(m.player.Volume() - 5)
			m.cfg.Volume = m.player.Volume()
			m.persist()
			return m, nil
		case key.Matches(msg, keys.Mute):
			m.player.ToggleMute()
			return m, nil
		case key.Matches(msg, keys.SeekFwd):
			m.player.SeekBy(5 * time.Second)
			return m, nil
		case key.Matches(msg, keys.SeekBack):
			m.player.SeekBy(-5 * time.Second)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			m.mode = modeMain
			return m, nil
		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, keys.Quit):
			m.persist()
			return m, tea.Quit
		case key.Matches(msg, keys.ApplyDir):
			dir := m.picker.CurrentDirectory
			m.cfg.MusicDir = dir
			m.persist()
			m.mode = modeMain
			m.stopPlayback()
			m.status = "library: " + dir
			return m, tea.Batch(m.loadLibrary(dir), clearStatusAfter(3*time.Second))
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

// View ------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.mode == modeSettings {
		return m.viewSettings()
	}
	return m.viewMain()
}

func (m Model) viewMain() string {
	lo := m.layout()
	m.list.SetSize(lo.listW, lo.listH)
	// At small heights the list chrome (status bar, pagination dots)
	// would push the list over budget (lipgloss Height pads, never
	// clips), so collapse it — the list stays fully navigable.
	compact := lo.listH < 8
	m.list.SetShowStatusBar(!compact)
	m.list.SetShowPagination(!compact)
	m.prog.Width = lo.progW
	m.help.Width = m.width
	m.help.ShowAll = m.showHelp

	header := m.viewHeader()
	card := m.viewCard(lo)

	var middle string
	if lo.wide {
		// Pad the list to its full column width so the song card
		// docks at the right edge (list View only spans its content).
		listBox := lipgloss.PlaceHorizontal(lo.listW, lipgloss.Left, m.list.View())
		card = lipgloss.Place(lo.rightW, lo.listH, lipgloss.Center, lipgloss.Top, card)
		middle = lipgloss.JoinHorizontal(lipgloss.Top, listBox, " ", card)
	} else {
		middle = m.list.View() + "\n" + card
	}

	var sb strings.Builder
	sb.WriteString(header + "\n")
	sb.WriteString(middle + "\n")
	sb.WriteString(m.viewControls(lo) + "\n")
	if m.errText != "" {
		sb.WriteString(errStyle.Render("! "+trunc(m.errText, max(m.width-4, 10))) + "\n")
	} else if m.status != "" {
		sb.WriteString(dimStyle.Render("* "+trunc(m.status, max(m.width-4, 10))) + "\n")
	}
	sb.WriteString(dimStyle.Render(m.help.View(keys)))
	return sb.String()
}

func (m Model) viewHeader() string {
	brand := brandStyle.Render("♪ GoMusic")
	dirMax := max(m.width-lipgloss.Width(brand)-2, 0)
	return brand + dimStyle.Render(" "+trunc(m.cfg.MusicDir, dirMax))
}

// viewCard renders the song card: cover art + song name + format.
func (m Model) viewCard(lo layout) string {
	// Song card: cover art + song name. Content width depends on mode.
	cw := m.width
	if lo.wide {
		cw = max(lo.rightW-2, 10)
	}
	cw = max(cw, 10)

	var sb strings.Builder
	if lo.showArt {
		art := m.artText
		if art == "" {
			art = placeholderArt(lo.artW, lo.artH)
		}
		sb.WriteString(lipgloss.PlaceHorizontal(cw, lipgloss.Center, art) + "\n")
	}

	if m.cur >= 0 && m.cur < len(m.tracks) {
		t := m.tracks[m.cur]
		icon := "\u25b6"
		if m.player.Paused() || !m.playing {
			icon = "\u23f8"
		}
		title := lipgloss.PlaceHorizontal(cw, lipgloss.Center,
			nowStyle.Render(trunc(icon+" "+t.Title, cw)))
		sb.WriteString(title + "\n")
		sub := t.Artist
		if t.Album != "" {
			sub += " \u2014 " + t.Album
		}
		if sub == "" {
			sub = trunc(t.Path, cw)
		}
		sb.WriteString(lipgloss.PlaceHorizontal(cw, lipgloss.Center,
			dimStyle.Render(trunc(sub, cw))) + "\n")
	} else {
		sb.WriteString(lipgloss.PlaceHorizontal(cw, lipgloss.Center,
			dimStyle.Render("\u2014 nothing playing \u2014")) + "\n")
		sb.WriteString(lipgloss.PlaceHorizontal(cw, lipgloss.Center,
			dimStyle.Render(trunc("enter on a track to play", cw))) + "\n")
	}

	ext, rate, ch := m.player.Info()
	sb.WriteString(lipgloss.PlaceHorizontal(cw, lipgloss.Center,
		fmtStyle.Render(trunc(formatLine(ext, rate, ch), cw))))
	return sb.String()
}

// viewControls renders the full-width bottom bar: progress + time,
// then state, volume, toggles and queue position.
func (m Model) viewControls(lo layout) string {
	pos, length := m.player.Position(), m.player.Length()
	pct := 0.0
	if length > 0 {
		pct = clampFloat(float64(pos)/float64(length), 0, 1)
	}
	timeStr := fmtDur(pos) + " / " + fmtDur(length)
	m.prog.Width = max(m.width-len(timeStr)-1, 8)
	line1 := m.prog.ViewAs(pct) + " " + timeStr

	icon := "\u25a0"
	if m.cur >= 0 && m.playing {
		icon = "\u25b6"
		if m.player.Paused() {
			icon = "\u23f8"
		}
	}
	vol, muted := m.player.Volume(), m.player.Muted()
	volBadge := badgeOff.Render(fmt.Sprintf("VOL %d%%", vol))
	if muted {
		volBadge = badgeOn.Render("MUTE")
	}
	shuf := badgeOff.Render("SHUF")
	if m.cfg.Shuffle {
		shuf = badgeOn.Render("SHUF")
	}
	rep := badgeOff.Render("REP:" + repeatLabel(m.cfg.Repeat))
	if m.cfg.Repeat != config.RepeatOff {
		rep = badgeOn.Render("REP:" + repeatLabel(m.cfg.Repeat))
	}
	left := icon + " " + volumeBar(vol, muted, lo.volW) + " " + volBadge + " " + shuf + " " + rep
	queue := ""
	if len(m.tracks) > 0 && m.cur >= 0 {
		queue = queueStyle.Render(fmt.Sprintf("%d/%d", m.cur+1, len(m.tracks)))
	}
	line2 := left
	if queue != "" && lipgloss.Width(left)+lipgloss.Width(queue)+1 <= m.width {
		line2 += strings.Repeat(" ", m.width-lipgloss.Width(left)-lipgloss.Width(queue)) + queue
	}
	return line1 + "\n" + line2
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func volumeBar(vol int, muted bool, width int) string {
	if muted {
		vol = 0
	}
	width = max(width, 4)
	filled := vol * width / 100
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString("█")
		} else {
			b.WriteString("░")
		}
	}
	b.WriteString("]")
	return lipgloss.NewStyle().Foreground(accent2).Render(b.String())
}

func (m Model) viewSettings() string {
	m.picker.Height = max(m.height-10, 4)
	m.help.Width = m.width
	m.help.ShowAll = m.showHelp
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ settings"))
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Music folder: ") + trunc(m.picker.CurrentDirectory, max(m.width-16, 10)) + "\n")
	b.WriteString(m.picker.View())
	b.WriteString("\n")
	b.WriteString(boxStyle.Render(
		"Navigate: ↑/↓ or j/k • open: enter/l • up: h/backspace\n" +
			"Press a to use the current folder • esc to cancel",
	))
	b.WriteString("\n" + dimStyle.Render(m.help.View(keys)))
	return b.String()
}
