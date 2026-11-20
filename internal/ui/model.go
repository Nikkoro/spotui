// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"image"
	"strings"
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
	"github.com/Nikkoro/spotui/internal/termimg"
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
	tickInterval = 200 * time.Millisecond
	flashTTL     = 4 * time.Second

	minPixelSize = 8
	maxPixelSize = 64
	pixelStep    = 4
	miniCoverPx  = 8
)

// Model is the root Bubble Tea model.
type Model struct {
	statusCheckedAt time.Time
	npChoice        string // Now Playing layout picked: config, then L
	updateTag       string // a newer release, once the daily check found one
	set             settingsState
	npFocus         int // Now Playing section tab moved to, npFocus*
	npNextCursor    int // selected entry in Up next

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

	// real cover images (Sixel / kitty)
	img            *termimg.Writer
	caps           termimg.Caps
	imageMode      bool
	savedImageMode *bool
	imgPixels      int
	imgLevels      int // colour levels for real images; 0 keeps every colour
	imgCache       map[string]*image.RGBA
	frame          *imgFrame

	themeName     string
	dynamicAccent bool

	// cover art
	covers    *cover.Cache
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

	hits      *hitMap
	lastClick struct {
		at  time.Time
		idx int
	}

	overlay overlay
	pal     cmdPalette
	dj      djPanel
	picker  struct {
		list       listModel
		filter     textinput.Model
		trackURI   string
		trackTitle string
	}

	sleepStep       int
	sleepID         int
	sleepAt         time.Time
	sleepEndOfTrack bool
	devices         struct {
		list    listModel
		loading bool
		err     error
	}

	keymap        map[string]string
	searchHistory []string
	historyIdx    int

	// list filter (shared by all list pages)
	filtering bool
	filterIn  textinput.Model

	// "play a link" prompt
	input     textinput.Model
	inputting bool

	// Web API authorization in progress, and the library tab waiting for it
	webAuthURL    string
	pendingLibTab int

	notifier  notify.Notifier
	notifyOff bool

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
	vizOn       bool
	vizStyle    string
	vizWave     []float32
	vizWaveGain float64
	vizTap      *viz.Tap
	vizAn       *viz.Analyzer
	vizBuf      []float32
	vizLevels   []float64
	vizPeaks    []float64
	vizAt       time.Time

	warnings       []string
	restart        bool
	restartAttempt int
	restartAt      time.Time

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
		eng:           eng,
		cfg:           cfg,
		phase:         phaseConnecting,
		spinner:       sp,
		theme:         newTheme(defaultAccent),
		volumeMax:     100,
		stopped:       true,
		liked:         map[string]bool{},
		input:         in,
		filterIn:      filter,
		pixSize:       clamp(cfg.Cover.PixelSize, minPixelSize, maxPixelSize),
		levels:        cfg.Cover.Levels,
		page:          pageNowPlaying,
		lib:           newLibraryPage(),
		search:        newSearchPage(),
		queue:         newQueuePage(),
		dj:            newDjPanel(),
		pal:           newPalette(),
		hits:          &hitMap{},
		frame:         &imgFrame{},
		imgCache:      map[string]*image.RGBA{},
		imgPixels:     cfg.Cover.ImagePixels,
		themeName:     cfg.Theme,
		dynamicAccent: cfg.Cover.DynamicAccent,
		vizOn:         cfg.Visualizer.Enabled && tap != nil,
		vizTap:        tap,
		vizStyle:      cfg.Visualizer.Style,
		npChoice:      cfg.NowPlaying.Layout,
		vizAn:         viz.NewAnalyzer(32),
		vizBuf:        make([]float32, 2048),
	}
	m.covers = cover.NewCache(cfg.CacheDir+"/covers", int64(cfg.Cover.CacheSizeMB)<<20)
	go m.covers.Prune()

	m.historyIdx = -1
	m.pendingLibTab = -1
	m.keymap, _ = buildKeymap(cfg.Keys)
	m.picker.filter = textinput.New()
	m.picker.filter.Prompt = "/ "
	m.picker.filter.Placeholder = "playlist name"
	if !validNpLayout(m.npChoice) {
		m.warnings = append(m.warnings, "now_playing.layout \""+m.npChoice+"\" is not one of "+strings.Join(npLayouts, ", "))
		m.npChoice = npAuto
	}
	m.restoreState()

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
type prefetchDueMsg struct{}

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

func (m Model) fetchCover(url string) tea.Cmd {
	covers := m.covers
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		img, err := covers.Get(ctx, url)
		return coverMsg{url: url, img: img, err: err}
	}
}

// prefetchNextCover warms the cover cache for the upcoming track, so the
// next track change shows its cover at once.
func (m Model) prefetchNextCover() tea.Cmd {
	eng, covers := m.eng, m.covers
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if s, err := eng.Status(ctx); err == nil && s.NextTrack != nil && s.NextTrack.AlbumCoverUrl != nil {
			covers.Prefetch(*s.NextTrack.AlbumCoverUrl)
		}
		return nil
	}
}

// fetchLikedMany asks for the hearts of many tracks, in batches.
func (m Model) fetchLikedMany(uris []string) tea.Cmd {
	if len(uris) == 0 {
		return nil
	}
	eng := m.eng
	return func() tea.Msg {
		all := map[string]bool{}
		for start := 0; start < len(uris); start += 100 {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			states, err := eng.Liked(ctx, uris[start:min(start+100, len(uris))])
			cancel()
			if err != nil {
				break
			}
			for k, v := range states {
				all[k] = v
			}
		}
		return likedMsg{states: all}
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
		if m.img != nil {
			m.img.Invalidate() // the renderer repaints everything
		}
		if m.phase == phaseReady && len(m.queue.list.items) == 0 && !m.queue.loading {
			return m, m.loadQueueForNp() // grown into the layout that shows it
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		// Now and then make sure the screen shows what the player is on, in
		// case a track change was missed. Asks the embedded player, not Spotify.
		if m.phase == phaseReady && time.Since(m.statusCheckedAt) >= statusCheckEvery {
			m.statusCheckedAt = time.Now()
			return m, tea.Batch(tick(), m.refreshStatus(0))
		}
		return m, tick()

	case vizTickMsg:
		return m, m.stepVisualizer(time.Time(msg))

	case engineMsg:
		cmd := m.handleEvent(msg.ev)
		return m, tea.Batch(cmd, waitEvent(m.eng.Events()))

	case updateMsg:
		m.handleUpdateMsg(msg)
		return m, nil

	case updateCheckedMsg, updateInstalledMsg:
		m.handleUpdateResult(msg)
		return m, nil

	case statusMsg:
		if msg.s != nil && msg.s.Track != nil && m.trackStale(msg.s.Track.Uri) {
			m.eng.Debugf("ui: status brought %s, was showing %s", msg.s.Track.Uri, m.currentURI())
			return m, m.applyStatus(msg.s)
		}
		return m, nil

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

	case restartDueMsg:
		if m.phase == phaseFatal {
			return m.restartNow()
		}
		return m, nil

	case sleepDueMsg:
		if msg.id != m.sleepID || m.sleepAt.IsZero() {
			return m, nil
		}
		return m, m.sleepNow()

	case flashMsg:
		m.setFlash(msg.text, msg.err)
		return m, nil

	case prefetchDueMsg:
		return m, m.prefetchNextCover()

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
		case m.overlay == overlayPlaylists:
			return m.updatePickerKey(msg)
		case m.overlay == overlayPalette:
			return m.updatePaletteKey(msg)
		case m.overlay == overlaySettings:
			return m.updateSettingsKey(msg)
		case msg.String() == "ctrl+k" && m.phase == phaseReady && !m.inputting:
			return m, m.openPalette()
		case m.overlay != overlayNone:
			return m.updateOverlayKey(msg.String())
		case m.inputting:
			return m.updateInput(msg)
		case m.filtering:
			return m.updateFilter(msg)
		case m.page == pageSearch && m.search.input.Focused():
			return m.updateSearchInput(msg)
		}
		return m.handleKey(m.remapKey(msg))
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
		cmds := []tea.Cmd{tea.EnableMouseCellMotion, m.applyStatus(ev.Status), m.checkUpdate()}
		if len(m.warnings) > 0 {
			m.setFlash("config.toml: "+strings.Join(m.warnings, " · "), true)
			m.warnings = nil
		}
		// A page restored from the last run needs its data now.
		switch m.page {
		case pageLibrary:
			cmds = append(cmds, m.ensureLibrary())
		case pageQueue:
			m.queue.loading = true
			cmds = append(cmds, m.loadQueue())
		}
		return tea.Batch(cmds...)

	case engine.FatalEvent:
		m.phase = phaseFatal
		m.fatal = ev.Err
		m.overlay = overlayNone
		return m.scheduleRestart()

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
		m.eng.Debugf("ui: metadata %s %q", t.Uri, t.Name)
		cmds = append(cmds, m.notifyTrack(&t), m.setTrack(&t))
	case *daemon.ApiEventDataMetadata:
		t := daemon.ApiTrack(*d)
		m.eng.Debugf("ui: metadata %s %q", t.Uri, t.Name)
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
		m.setFlash(playbackErrorText(d), true)
	case daemon.ApiEventDataPlaying:
		m.contextURI = d.ContextUri
		if m.trackStale(d.Uri) {
			// Playing something other than what is shown: the metadata event
			// went missing, so ask the player what it is on.
			m.eng.Debugf("ui: playing %s but showing %s, refreshing status", d.Uri, m.currentURI())
			cmds = append(cmds, m.refreshStatus(0))
		}
	case daemon.ApiEventDataWillPlay:
		m.contextURI = d.ContextUri
		if m.trackStale(d.Uri) {
			// The metadata event normally follows once the track has loaded;
			// check back in case it does not.
			cmds = append(cmds, m.refreshStatus(statusRecheckDelay))
		}
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
	case daemon.ApiEventTypeQueue, daemon.ApiEventTypeMetadata:
		if m.page == pageQueue {
			cmds = append(cmds, m.loadQueue())
		} else {
			cmds = append(cmds, m.loadQueueForNp())
		}
	}
	return tea.Batch(cmds...)
}

// statusRecheckDelay is how long after a track starts loading the UI checks
// that it was told about it.
const statusRecheckDelay = 2500 * time.Millisecond

// statusCheckEvery is how often the shown track is checked against the player.
const statusCheckEvery = 5 * time.Second

type statusMsg struct{ s *daemon.ApiStatus }

func (m Model) trackStale(uri string) bool {
	return uri != "" && (m.track == nil || m.track.Uri != uri)
}

// refreshStatus reads the player's state (in-process, no network) after d.
func (m Model) refreshStatus(d time.Duration) tea.Cmd {
	e := m.eng
	return func() tea.Msg {
		if d > 0 {
			time.Sleep(d)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s, err := e.Status(ctx)
		if err != nil {
			e.Debugf("ui: status refresh failed: %v", err)
			return nil
		}
		return statusMsg{s}
	}
}

func (m *Model) setTrack(t *daemon.ApiTrack) tea.Cmd {
	changed := m.track == nil || m.track.Uri != t.Uri
	// "Sleep at the end of this track": the next track is the cue.
	if changed && m.track != nil && m.sleepEndOfTrack {
		return m.sleepNow()
	}
	m.track = t
	m.setPosition(t.Position)

	var cmds []tea.Cmd
	if changed {
		cmds = append(cmds, m.fetchLiked(t.Uri), m.fetchLyrics(t))
		if m.cfg.Cover.Enabled {
			cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return prefetchDueMsg{} }))
		}
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
			cmds = append(cmds, m.fetchCover(url))
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
	m.applyAccent()
}

// applyAccent rebuilds the styles with the cover's colour or the palette's.
func (m *Model) applyAccent() {
	accent := defaultAccent
	if m.dynamicAccent && m.coverImg != nil {
		// Sample the accent at a fixed resolution so it does not shift when
		// the user changes the pixelation.
		if c, ok := cover.Accent(cover.Pixelate(m.coverImg, 24, 0)); ok {
			accent = cover.Hex(c)
		}
	}
	m.theme = newTheme(accent)
	m.spinner.Style = m.theme.accentTxt
}

// setTheme switches the palette live.
func (m *Model) setTheme(name string) {
	if !ApplyPalette(name) {
		return
	}
	m.themeName = name
	m.applyAccent()
	m.setFlash("Theme: "+name, false)
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
	ttl := flashTTL
	if isErr {
		ttl *= 2 // errors usually need reading twice
	}
	m.flashUntil = time.Now().Add(ttl)
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
	case phaseFatal:
		if key == "R" || key == "r" || key == "enter" {
			return m.restartNow()
		}
		return m, nil
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
	case ":":
		return m, m.openPalette()
	case "?":
		return m, m.openOverlay(overlayHelp)
	case "d":
		return m, m.openOverlay(overlayDevices)
	case "D":
		return m, m.djKey()
	case "P":
		return m, m.openPlaylistPicker()
	case "A":
		return m, m.goToAlbum()
	case "I":
		return m, m.goToArtist()
	case "z":
		return m, m.cycleSleep()
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
		step := m.seekStepMs()
		m.setPosition(m.position() + step)
		return m, call("Seek", func(ctx context.Context) error { return e.Seek(ctx, step, true) })
	case "left", "h":
		step := m.seekStepMs()
		m.setPosition(max(m.position()-step, 0))
		return m, call("Seek", func(ctx context.Context) error { return e.Seek(ctx, -step, true) })
	case "+", "=":
		step := m.volumeStep()
		return m, call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, step, true) })
	case "-", "_":
		step := m.volumeStep()
		return m, call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, -step, true) })
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
	case ",":
		return m, m.openSettings()
	case "L":
		m.cycleNpLayout()
		return m, m.loadQueueForNp()
	case "V":
		switch m.vizStyle {
		case "mirror":
			m.vizStyle = "wave"
		case "wave":
			m.vizStyle = "bars"
		default:
			m.vizStyle = "mirror"
		}
		m.vizWaveGain = 1
		m.vizOn = m.vizTap != nil
		m.setFlash("Visualizer style: "+m.vizStyle, false)
		return m, nil
	case "u":
		m.inputting = true
		m.input.SetValue("")
		return m, m.input.Focus()
	case "c":
		if m.img == nil || m.img.Protocol() == termimg.Blocks {
			m.setFlash("This terminal shows no images; covers stay pixel blocks", false)
			return m, nil
		}
		m.imageMode = !m.imageMode
		if m.imageMode {
			m.setFlash("Cover: image ("+m.img.Protocol().String()+")", false)
		} else {
			m.setFlash("Cover: pixel blocks", false)
		}
		return m, tea.ClearScreen
	case "]":
		if m.imagesOn() {
			m.imgPixels = nextImagePixels(m.imgPixels, +1)
			m.setFlash("Cover: "+imagePixelsLabel(m.imgPixels), false)
			return m, nil
		}
		m.pixSize = clamp(m.pixSize+pixelStep, minPixelSize, maxPixelSize)
		m.repixelate()
		m.setFlash(fmt.Sprintf("Cover resolution %d×%d", m.pixSize, m.pixSize), false)
	case "[":
		if m.imagesOn() {
			m.imgPixels = nextImagePixels(m.imgPixels, -1)
			m.setFlash("Cover: "+imagePixelsLabel(m.imgPixels), false)
			return m, nil
		}
		m.pixSize = clamp(m.pixSize-pixelStep, minPixelSize, maxPixelSize)
		m.repixelate()
		m.setFlash(fmt.Sprintf("Cover resolution %d×%d", m.pixSize, m.pixSize), false)
	case "}":
		if m.imagesOn() {
			m.imgLevels = nextLevels(m.imgLevels, +1)
			m.setFlash("Cover colours: "+levelsLabel(m.imgLevels), false)
			return m, nil
		}
		m.levels = nextLevels(m.levels, +1)
		m.repixelate()
		m.setFlash("Cover colours: "+levelsLabel(m.levels), false)
	case "{":
		if m.imagesOn() {
			m.imgLevels = nextLevels(m.imgLevels, -1)
			m.setFlash("Cover colours: "+levelsLabel(m.imgLevels), false)
			return m, nil
		}
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

func (m Model) seekStepMs() int64 {
	return int64(clamp(m.cfg.SeekStepSeconds, 1, 120)) * 1000
}

func (m Model) volumeStep() int32 {
	return int32(clamp(m.cfg.VolumeStep, 1, 50))
}

func clamp(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

// playbackErrorText turns a playback error into something actionable.
func playbackErrorText(d daemon.ApiEventDataPlaybackError) string {
	msg := strings.ToLower(d.Message)
	switch {
	case strings.Contains(msg, "alsa") || strings.Contains(msg, "pulse") || strings.Contains(msg, "wasapi") ||
		strings.Contains(msg, "audio device") || strings.Contains(msg, "output"):
		return "Can't open the audio device: " + d.Message + " · try another audio_backend in config.toml (spotui paths)"
	case d.Kind == "restricted" || d.Unplayable:
		return "This track can't be played here (region or account restriction); skipping"
	case d.Kind == "network" || d.Kind == "timeout":
		return "Network trouble while loading the track: " + d.Message
	}
	return "Playback error: " + d.Message
}
