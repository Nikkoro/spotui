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
	"github.com/Nikkoro/spotui/internal/platform"
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
)

// Model is the root Bubble Tea model.
type Model struct {
	eng *engine.Engine
	cfg *config.Config

	width, height int
	phase         phase
	spinner       spinner.Model
	theme         theme

	auth *daemon.ApiDeviceAuth

	track       *daemon.ApiTrack
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

	coverURL string
	coverImg image.Image
	coverPix *cover.Pixels
	pixSize  int
	levels   int

	input     textinput.Model
	inputting bool

	flash      string
	flashErr   bool
	flashUntil time.Time
	fatal      error
}

// New builds the UI around a running engine.
func New(eng *engine.Engine, cfg *config.Config) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))

	in := textinput.New()
	in.Placeholder = "spotify:album:… or https://open.spotify.com/…"
	in.Prompt = "▶ "
	in.CharLimit = 256

	m := Model{
		eng:       eng,
		cfg:       cfg,
		phase:     phaseConnecting,
		spinner:   sp,
		theme:     newTheme(defaultAccent),
		volumeMax: 100,
		stopped:   true,
		input:     in,
		pixSize:   clamp(cfg.Cover.PixelSize, minPixelSize, maxPixelSize),
		levels:    cfg.Cover.Levels,
	}
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
func (m Model) call(what string, fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

// Init starts listening to the engine.
func (m Model) Init() tea.Cmd {
	return tea.Batch(waitEvent(m.eng.Events()), m.spinner.Tick, tick())
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

	case engineMsg:
		cmd := m.handleEvent(msg.ev)
		return m, tea.Batch(cmd, waitEvent(m.eng.Events()))

	case cmdResultMsg:
		if msg.err != nil {
			m.setFlash(fmt.Sprintf("%s failed: %v", msg.what, msg.err), true)
		}
		return m, nil

	case coverMsg:
		if msg.url != m.coverURL {
			return m, nil // a newer track already replaced it
		}
		if msg.err != nil {
			m.coverImg, m.coverPix = nil, nil
			return m, nil
		}
		m.coverImg = msg.img
		m.repixelate()
		return m, nil

	case tea.KeyMsg:
		if m.inputting {
			return m.updateInput(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
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
		return m.applyStatus(ev.Status)

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
	if s.Track != nil {
		return m.setTrack(s.Track)
	}
	return nil
}

func (m *Model) applyPlayerEvent(ev engine.PlayerEvent) tea.Cmd {
	switch d := ev.Data.(type) {
	case daemon.ApiEventDataMetadata:
		t := daemon.ApiTrack(d)
		return m.setTrack(&t)
	case *daemon.ApiEventDataMetadata:
		t := daemon.ApiTrack(*d)
		return m.setTrack(&t)
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
	}
	return nil
}

func (m *Model) setTrack(t *daemon.ApiTrack) tea.Cmd {
	m.track = t
	m.setPosition(t.Position)

	url := ""
	if t.AlbumCoverUrl != nil {
		url = *t.AlbumCoverUrl
	}
	if url == m.coverURL {
		return nil
	}
	m.coverURL = url
	m.coverImg, m.coverPix = nil, nil
	m.theme = newTheme(defaultAccent)
	if url == "" || !m.cfg.Cover.Enabled {
		return nil
	}
	return fetchCover(url)
}

func (m *Model) repixelate() {
	if m.coverImg == nil {
		return
	}
	m.coverPix = cover.Pixelate(m.coverImg, m.pixSize, m.levels)
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

	e := m.eng
	switch key {
	case " ":
		// Optimistic toggle; the daemon's event confirms it right after.
		m.setPosition(m.position())
		m.paused = !m.paused
		return m, m.call("Play/pause", e.PlayPause)
	case "n":
		return m, m.call("Next", e.Next)
	case "p":
		return m, m.call("Previous", e.Prev)
	case "right", "l":
		m.setPosition(m.position() + seekStepMs)
		return m, m.call("Seek", func(ctx context.Context) error { return e.Seek(ctx, seekStepMs, true) })
	case "left", "h":
		m.setPosition(max(m.position()-seekStepMs, 0))
		return m, m.call("Seek", func(ctx context.Context) error { return e.Seek(ctx, -seekStepMs, true) })
	case "+", "=":
		return m, m.call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, volumeStep, true) })
	case "-", "_":
		return m, m.call("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, -volumeStep, true) })
	case "s":
		on := !m.shuffle
		return m, m.call("Shuffle", func(ctx context.Context) error { return e.SetShuffle(ctx, on) })
	case "r":
		// off -> context -> track -> off
		ctx, track := false, false
		switch {
		case !m.repeatCtx && !m.repeatTrack:
			ctx = true
		case m.repeatCtx && !m.repeatTrack:
			track = true
		}
		return m, m.call("Repeat", func(c context.Context) error { return e.SetRepeat(c, ctx, track) })
	case "u", "/":
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
		return m, m.call("Play", func(ctx context.Context) error { return m.eng.Play(ctx, uri) })
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
