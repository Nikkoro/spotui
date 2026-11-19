// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/cover"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/notify"
	"github.com/Nikkoro/spotui/internal/platform"
	"github.com/Nikkoro/spotui/internal/viz"
	"github.com/Nikkoro/spotui/internal/webapi"
)

type phase int

const (
	phaseConnecting phase = iota
	phaseAuth
	phaseReady
	phaseFatal
)

const (
	seekStepMs   = 5000
	volumeStep   = 5
	tickInterval = 200 * time.Millisecond
	flashTTL     = 4 * time.Second

	minPixelSize = 8
	maxPixelSize = 64
	pixelStep    = 4
	miniCoverPx  = 8
)

// Model is the root Bubble Tea model.
type Model struct {
	eng  *engine.Engine
	cfg  *config.Config
	api  *webapi.Client
	pkce *webapi.PKCEAuth // nil without a client id

	width, height int
	phase         phase
	spinner       spinner.Model
	theme         theme

	auth *daemon.ApiDeviceAuth

	// player state
	track       *daemon.ApiTrack
	contextURI  string
	stopped     bool
	paused      bool
	buffering   bool
	posMs       int64
	posAt       time.Time
	volume      uint32
	volumeMax   uint32
	shuffle     bool
	repeatCtx   bool
	repeatTrack bool
	username    string
	liked       map[string]bool

	// cover art
	coverURL  string
	coverImg  image.Image
	coverPix  *cover.Pixels
	coverMini *cover.Pixels
	// coverFit caches covers shrunk to fit small terminals, by size. It is
	// a map so View (a value receiver) can fill it.
	coverFit map[int]*cover.Pixels
	pixSize  int
	levels   int

	// pages
	page    page
	lib     libraryPage
	search  searchPage
	queue   queuePage
	ctxPage *contextPage

	overlay overlay
	dj      djPanel
	devices struct {
		list    listModel
		loading bool
		err     error
	}

	// list filter (shared by all list pages)
	filtering bool
	filterIn  textinput.Model

	// "play a link" prompt
	input     textinput.Model
	inputting bool

	// Web API authorization in progress
	webAuthURL string

	notifier notify.Notifier

	// lyrics
	lyr           *lyrics.Client
	lyrics        *lyrics.Lyrics
	lyricsURI     string
	lyricsTrack   *daemon.ApiTrack
	lyricsLoading bool
	lyricsErr     error
	lyricsOffset  time.Duration
	lyricsScroll  int

	// visualizer
	vizOn      bool
	vizStyle   string
	vizTap     *viz.Tap
	vizAn      *viz.Analyzer
	vizBuf     []float32
	vizLevels  []float64
	vizPeaks   []float64
	vizSeen    int64
	vizStalled int
	vizAt      time.Time

	flash      string
	flashErr   bool
	flashUntil time.Time
	fatal      error
}

// New builds the UI around a running engine.
func New(eng *engine.Engine, cfg *config.Config, tap *viz.Tap) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))

	in := textinput.New()
	in.Placeholder = "spotify:album:… or https://open.spotify.com/…"
	in.Prompt = "▶ "
	in.CharLimit = 256

	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "filter"
	filter.CharLimit = 64

	m := Model{
		eng:       eng,
		cfg:       cfg,
		phase:     phaseConnecting,
		spinner:   sp,
		theme:     newTheme(defaultAccent),
		volumeMax: 100,
		stopped:   true,
		liked:     map[string]bool{},
		input:     in,
		filterIn:  filter,
		pixSize:   clamp(cfg.Cover.PixelSize, minPixelSize, maxPixelSize),
		levels:    cfg.Cover.Levels,
		page:      pageNowPlaying,
		lib:       newLibraryPage(),
		search:    newSearchPage(),
		queue:     newQueuePage(),
		dj:        newDjPanel(),
		vizOn:     cfg.Visualizer.Enabled && tap != nil,
		vizTap:    tap,
		vizStyle:  cfg.Visualizer.Style,
		vizAn:     viz.NewAnalyzer(32),
		vizBuf:    make([]float32, 2048),
	}
	if cfg.Notifications {
		m.notifier = notify.New(config.IsWSL())
	}
	if cfg.Lyrics.Enabled {
		m.lyr = lyrics.NewClient(cfg.CacheDir+"/lyrics", Version)
	}

	var tokens webapi.TokenSource = webapi.TokenFunc(eng.Token)
	if cfg.ClientID != "" {
		m.pkce = webapi.NewPKCEAuth(cfg.ClientID, cfg.ConfigDir+"/webapi_token.json")
		tokens = m.pkce
	}
	m.api = webapi.NewClient(tokens)

	m.spinner.Style = m.theme.accentTxt
	return m
}

// --- messages ---

type engineMsg struct{ ev engine.Event }
type tickMsg time.Time
type cmdResultMsg struct {
	what string
	err  error
}
type coverMsg struct {
	url string
	img image.Image
	err error
}
type likedMsg struct {
	states map[string]bool
}

func waitEvent(ch <-chan engine.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return engineMsg{ev}
	}
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// call runs an engine command off the UI goroutine.
func call(what string, fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return cmdResultMsg{what: what, err: fn(ctx)}
	}
}

func fetchCover(url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		img, err := cover.Fetch(ctx, url)
		return coverMsg{url: url, img: img, err: err}
	}
}

func (m Model) fetchLiked(uri string) tea.Cmd {
	if uri == "" {
		return nil
	}
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		states, err := eng.Liked(ctx, []string{uri})
		if err != nil {
			return nil
		}
		return likedMsg{states: states}
	}
}

// Init starts listening to the engine.
func (m Model) Init() tea.Cmd {
	return tea.Batch(waitEvent(m.eng.Events()), m.spinner.Tick, tick(), vizTick(vizIdleInterval))
}

// --- update ---

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		return m, tick()

	case vizTickMsg:
		return m, m.stepVisualizer(time.Time(msg))

	case engineMsg:
		cmd := m.handleEvent(msg.ev)
		return m, tea.Batch(cmd, waitEvent(m.eng.Events()))

	case djSuggestionsMsg:
		m.setDjSuggestions(msg)
		return m, nil

	case devicesMsg:
		m.devices.loading = false
		m.devices.err = msg.err
		if msg.err == nil {
			m.devices.list.setItems(msg.items)
		}
		return m, nil

	case cmdResultMsg:
		if msg.err != nil {
			m.setFlash(fmt.Sprintf("%s failed: %v", msg.what, msg.err), true)
		} else if msg.what == "Add to queue" {
			m.setFlash("Added to queue", false)
			if m.page == pageQueue {
				return m, m.loadQueue()
			}
		}
		return m, nil

	case notifyDueMsg:
		return m, m.sendNotification(msg.uri)

	case lyricsMsg, lyricsDueMsg:
		return m, m.handleLyricsMsg(msg)

	case likedMsg:
		for k, v := range msg.states {
			m.liked[k] = v
		}
		return m, nil

	case coverMsg:
		if msg.url != m.coverURL {
			return m, nil // a newer track already replaced it
		}
		if msg.err != nil {
			m.coverImg, m.coverPix, m.coverMini = nil, nil, nil
			return m, nil
		}
		m.coverImg = msg.img
		m.repixelate()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		switch {
		case m.overlay == overlayDj:
			return m.updateDjKey(msg)
		case m.overlay != overlayNone:
			return m.updateOverlayKey(msg.String())
		case m.inputting:
			return m.updateInput(msg)
		case m.filtering:
			return m.updateFilter(msg)
		case m.page == pageSearch && m.search.input.Focused():
			return m.updateSearchInput(msg)
		}
		return m.handleKey(msg)
	}

	return m.updatePages(msg)
}

func (m *Model) handleEvent(ev engine.Event) tea.Cmd {
	switch ev := ev.(type) {
	case engine.AuthEvent:
		if ev.Pending != nil {
			m.auth = ev.Pending
			m.phase = phaseAuth
		} else {
			m.auth = nil
			if m.phase == phaseAuth {
				m.phase = phaseConnecting
			}
		}

	case engine.ReadyEvent:
		m.phase = phaseReady
		m.auth = nil
		return tea.Batch(tea.EnableMouseCellMotion, m.applyStatus(ev.Status))

	case engine.FatalEvent:
		m.phase = phaseFatal
		m.fatal = ev.Err

	case engine.PlayerEvent:
		if m.phase != phaseReady {
			m.phase = phaseReady
		}
		return m.applyPlayerEvent(ev)
	}
	return nil
}

func (m *Model) applyStatus(s *daemon.ApiStatus) tea.Cmd {
	if s == nil {
		return nil
	}
	m.username = s.Username
	m.paused = s.Paused
	m.stopped = s.Stopped
	m.buffering = s.Buffering
	m.shuffle = s.ShuffleContext
	m.repeatCtx = s.RepeatContext
	m.repeatTrack = s.RepeatTrack
	m.volume = s.Volume
	if s.VolumeSteps > 0 {
		m.volumeMax = s.VolumeSteps
	}
	if s.ContextUri != nil {
		m.contextURI = *s.ContextUri
	}
	if s.Track != nil {
		return m.setTrack(s.Track)
	}
	return nil
}

func (m *Model) applyPlayerEvent(ev engine.PlayerEvent) tea.Cmd {
	var cmds []tea.Cmd
	switch d := ev.Data.(type) {
	case daemon.ApiEventDataMetadata:
		t := daemon.ApiTrack(d)
		cmds = append(cmds, m.notifyTrack(&t), m.setTrack(&t))
	case *daemon.ApiEventDataMetadata:
		t := daemon.ApiTrack(*d)
		cmds = append(cmds, m.notifyTrack(&t), m.setTrack(&t))
	case daemon.ApiEventDataSeek:
		m.setPosition(int64(d.Position))
	case daemon.ApiEventDataVolume:
		m.volume = d.Value
		if d.Max > 0 {
			m.volumeMax = d.Max
		}
	case daemon.ApiEventDataShuffleContext:
		m.shuffle = d.Value
	case daemon.ApiEventDataRepeatContext:
		m.repeatCtx = d.Value
	case daemon.ApiEventDataRepeatTrack:
		m.repeatTrack = d.Value
	case daemon.ApiEventDataPlaybackError:
		m.setFlash("Playback error: "+d.Message, true)
	case daemon.ApiEventDataPlaying:
		m.contextURI = d.ContextUri
	case daemon.ApiEventDataWillPlay:
		m.contextURI = d.ContextUri
	}

	switch ev.Type {
	case daemon.ApiEventTypePlaying:
		m.setPosition(m.position())
		m.paused, m.stopped, m.buffering = false, false, false
	case daemon.ApiEventTypePaused:
		m.setPosition(m.position())
		m.paused, m.buffering = true, false
	case daemon.ApiEventTypeWillPlay:
		m.buffering = true
	case daemon.ApiEventTypeNotPlaying, daemon.ApiEventTypeStopped:
		m.stopped, m.buffering = true, false
	case daemon.ApiEventTypeInactive:
		m.setFlash("Playback moved to another device", false)
	case daemon.ApiEventTypeActive:
		m.setFlash("Playing on this terminal", false)
	case daemon.ApiEventTypeQueue:
		if m.page == pageQueue {
			cmds = append(cmds, m.loadQueue())
		}
	case daemon.ApiEventTypeMetadata:
		if m.page == pageQueue {
			cmds = append(cmds, m.loadQueue())
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) setTrack(t *daemon.ApiTrack) tea.Cmd {
	changed := m.track == nil || m.track.Uri != t.Uri
	m.track = t
	m.setPosition(t.Position)

	var cmds []tea.Cmd
	if changed {
		cmds = append(cmds, m.fetchLiked(t.Uri), m.fetchLyrics(t))
	}

	url := ""
	if t.AlbumCoverUrl != nil {
		url = *t.AlbumCoverUrl
	}
	if url != m.coverURL {
		m.coverURL = url
		m.coverImg, m.coverPix, m.coverMini = nil, nil, nil
		m.theme = newTheme(defaultAccent)
		if url != "" && m.cfg.Cover.Enabled {
			cmds = append(cmds, fetchCover(url))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) repixelate() {
	if m.coverImg == nil {
		return
	}
	m.coverPix = cover.Pixelate(m.coverImg, m.pixSize, m.levels)
	m.coverFit = map[int]*cover.Pixels{}
	m.coverMini = cover.Pixelate(m.coverImg, miniCoverPx, m.levels)
	accent := defaultAccent
	if m.cfg.Cover.DynamicAccent {
		// Sample the accent at a fixed resolution so it does not shift when
		// the user changes the pixelation.
		if c, ok := cover.Accent(cover.Pixelate(m.coverImg, 24, 0)); ok {
			accent = cover.Hex(c)
		}
	}
	m.theme = newTheme(accent)
	m.spinner.Style = m.theme.accentTxt
}

func (m *Model) setPosition(ms int64) {
	m.posMs = ms
	m.posAt = time.Now()
}

// position extrapolates the playback position between events.
func (m Model) position() int64 {
	pos := m.posMs
	if !m.paused && !m.stopped && !m.buffering && !m.posAt.IsZero() {
		pos += time.Since(m.posAt).Milliseconds()
	}
	if m.track != nil && pos > int64(m.track.Duration) {
		pos = int64(m.track.Duration)
	}
	return max(pos, 0)
}

func (m *Model) setFlash(text string, isErr bool) {
	m.flash = text
	m.flashErr = isErr
	m.flashUntil = time.Now().Add(flashTTL)
}

func (m Model) currentURI() string {
	if m.track == nil {
		return ""
	}
	return m.track.Uri
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	switch m.phase {
	case phaseAuth:
		if key == "o" && m.auth != nil {
			if err := platform.OpenURL(m.auth.Url); err != nil {
				m.setFlash("Could not open browser: "+err.Error(), true)
			}
		}
		return m, nil
	case phaseReady:
	default:
		return m, nil
	}

	switch key {
	case "?":
		return m, m.openOverlay(overlayHelp)
	case "d":
		return m, m.openOverlay(overlayDevices)
	case "D":
		return m, m.djKey()
	}

	// Page navigation and list keys first; they fall through to the
	// player keys when not handled.
	if handled, next, cmd := m.handlePageKey(key); handled {
		return next, cmd
	}

	e := m.eng
	switch key {
	case " ":
		// Optimistic toggle; the daemon's event confirms it right after.
		m.setPosition(m.position())
		m.paused = !m.paused
		return m, call("Play/pause", e.PlayPause)
	case "n":
		return m, call("Next", e.Next)
	case "p":
		return m, call("Previous", e.Prev)
	case "right", "l":
		m.setPosition(m.position() + seekStepMs)
		return m, call("Seek", func(ctx context.Context) error { return e.Seek(ctx, seekStepMs, true) })
	case "left", "h":
		m.setPosition(max(m.position()-seekStepMs, 0))
		return m, call("Seek", func(ctx context.Context) error { return e.Seek(ctx, -seekStepMs, true) })
	case "+", "=":
		return m, call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, volumeStep, true) })
	case "-", "_":
		return m, call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, -volumeStep, true) })
	case "s":
		on := !m.shuffle
		return m, call("Shuffle", func(ctx context.Context) error { return e.SetShuffle(ctx, on) })
	case "r":
		// off -> context -> track -> off
		rc, rt := false, false
		switch {
		case !m.repeatCtx && !m.repeatTrack:
			rc = true
		case m.repeatCtx && !m.repeatTrack:
			rt = true
		}
		return m, call("Repeat", func(ctx context.Context) error { return e.SetRepeat(ctx, rc, rt) })
	case "f":
		uri := m.currentURI()
		if uri == "" {
			return m, nil
		}
		on := !m.liked[uri]
		m.liked[uri] = on
		if on {
			m.setFlash("Added to Liked Songs", false)
		} else {
			m.setFlash("Removed from Liked Songs", false)
		}
		return m, call("Like", func(ctx context.Context) error { return e.SetLiked(ctx, []string{uri}, on) })
	case "v":
		if m.vizTap == nil {
			return m, nil
		}
		m.vizOn = !m.vizOn
		if m.vizOn {
			m.setFlash("Visualizer on", false)
		} else {
			m.setFlash("Visualizer off", false)
		}
		return m, nil
	case "V":
		if m.vizStyle == "mirror" {
			m.vizStyle = "bars"
		} else {
			m.vizStyle = "mirror"
		}
		m.vizOn = m.vizTap != nil
		m.setFlash("Visualizer style: "+m.vizStyle, false)
		return m, nil
	case "u":
		m.inputting = true
		m.input.SetValue("")
		return m, m.input.Focus()
	case "]":
		m.pixSize = clamp(m.pixSize+pixelStep, minPixelSize, maxPixelSize)
		m.repixelate()
		m.setFlash(fmt.Sprintf("Cover resolution %d×%d", m.pixSize, m.pixSize), false)
	case "[":
		m.pixSize = clamp(m.pixSize-pixelStep, minPixelSize, maxPixelSize)
		m.repixelate()
		m.setFlash(fmt.Sprintf("Cover resolution %d×%d", m.pixSize, m.pixSize), false)
	case "}":
		m.levels = nextLevels(m.levels, +1)
		m.repixelate()
		m.setFlash("Cover colours: "+levelsLabel(m.levels), false)
	case "{":
		m.levels = nextLevels(m.levels, -1)
		m.repixelate()
		m.setFlash("Cover colours: "+levelsLabel(m.levels), false)
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.inputting = false
		m.input.Blur()
		return m, nil
	case "enter":
		m.inputting = false
		m.input.Blur()
		uri, err := parseSpotifyURI(m.input.Value())
		if err != nil {
			m.setFlash(err.Error(), true)
			return m, nil
		}
		m.setFlash("Loading "+uri, false)
		eng := m.eng
		return m, call("Play", func(ctx context.Context) error { return eng.Play(ctx, uri) })
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// Colour level presets, from crunchy to full colour (0).
var levelPresets = []int{2, 3, 4, 6, 8, 12, 16, 0}

func nextLevels(cur, dir int) int {
	idx := len(levelPresets) - 1
	for i, v := range levelPresets {
		if v == cur {
			idx = i
			break
		}
	}
	return levelPresets[clamp(idx+dir, 0, len(levelPresets)-1)]
}

func levelsLabel(l int) string {
	if l <= 1 || l >= 256 {
		return "full"
	}
	return fmt.Sprintf("%d levels/channel", l)
}

func clamp(v, lo, hi int) int {
	return max(lo, min(hi, v))
}
