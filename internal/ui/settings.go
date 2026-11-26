package ui

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/notify"
	"github.com/Nikkoro/spotui/internal/update"
)

// The settings page (,) changes what config.toml holds without editing it
// by hand. Each change is applied at once where the running player allows
// and written to config.toml straight away, only its own line; the rest
// take effect when the player restarts, which R does from here.

type settingKind int

const (
	setBool settingKind = iota
	setChoice
	setInt
	setText
	setAction
)

type setting struct {
	group, label, help string
	kind               settingKind
	section, key       string   // where it lives in config.toml; "" for none
	options            []string // setChoice
	min, max, step     int      // setInt
	unit               string   // setInt, shown after the number
	restart            bool     // takes effect after a restart

	// value is the current value: "true"/"false", an option, a number or
	// text. set applies v and returns what to write to config.toml (nil:
	// nothing) and a command. run is for actions.
	value func(m Model) string
	set   func(m *Model, v string) (any, tea.Cmd)
	run   func(m *Model) tea.Cmd
	hide  func(m Model) bool
}

type settingsState struct {
	cursor  int
	scroll  int
	editing bool
	input   textinput.Model
	restart bool // a change waits for a restart
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func atoiOr(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func themeNames() []string {
	names := make([]string, 0, len(palettes))
	for n := range palettes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func vizDelayOptions() []string {
	opts := []string{"auto"}
	for d := 0; d <= 600; d += 20 {
		opts = append(opts, itoa(d))
	}
	return opts
}

func audioBackends() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"wasapi"}
	case "darwin":
		return []string{"audio-toolbox"}
	}
	return []string{"pulseaudio", "alsa"}
}

func (m Model) settings() []setting {
	c := m.cfg
	pixLabels := make([]string, len(imagePixelPresets))
	for i, p := range imagePixelPresets {
		pixLabels[i] = itoa(p)
	}
	s := []setting{
		// --- playback
		{group: "Playback", label: "Device name", kind: setText, key: "device_name", restart: true,
			help:  "How this terminal shows up in Spotify Connect.",
			value: func(m Model) string { return m.cfg.DeviceName },
			set: func(m *Model, v string) (any, tea.Cmd) {
				v = strings.TrimSpace(v)
				if v == "" {
					return nil, nil
				}
				m.cfg.DeviceName = v
				return v, nil
			}},
		{group: "Playback", label: "Audio quality", kind: setChoice, key: "bitrate", restart: true,
			options: []string{"96", "160", "320"}, unit: " kbps",
			help:  "Stream bitrate; 320 needs Premium.",
			value: func(m Model) string { return itoa(m.cfg.Bitrate) },
			set:   func(m *Model, v string) (any, tea.Cmd) { m.cfg.Bitrate = atoiOr(v, 320); return m.cfg.Bitrate, nil }},
		{group: "Playback", label: "Crossfade", kind: setInt, key: "crossfade_ms", restart: true,
			min: 0, max: 12000, step: 1000, unit: " ms",
			help:  "Blend the end of a track into the next. 0 is off.",
			value: func(m Model) string { return itoa(m.cfg.CrossfadeMs) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.CrossfadeMs = atoiOr(v, 0)
				return m.cfg.CrossfadeMs, nil
			}},
		{group: "Playback", label: "Autoplay", kind: setBool, key: "autoplay", restart: true,
			help:  "Keep similar music going when a playlist or album ends.",
			value: func(m Model) string { return boolStr(m.cfg.Autoplay) },
			set:   func(m *Model, v string) (any, tea.Cmd) { m.cfg.Autoplay = v == "true"; return m.cfg.Autoplay, nil }},
		{group: "Playback", label: "Loudness normalisation", kind: setBool, key: "normalisation", restart: true,
			help:  "Play every track at a similar loudness.",
			value: func(m Model) string { return boolStr(m.cfg.Normalisation) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Normalisation = v == "true"
				return m.cfg.Normalisation, nil
			}},
		{group: "Playback", label: "Audio output", kind: setChoice, key: "audio_backend", restart: true,
			options: audioBackends(),
			help:    "The sound system to play through.",
			hide:    func(Model) bool { return len(audioBackends()) < 2 },
			value:   func(m Model) string { return m.cfg.AudioBackend },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.cfg.AudioBackend = v; return v, nil }},
		{group: "Playback", label: "Seek step", kind: setInt, key: "seek_step_seconds",
			min: 1, max: 60, step: 1, unit: " s",
			help:  "How far ← and → jump.",
			value: func(m Model) string { return itoa(m.cfg.SeekStepSeconds) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.SeekStepSeconds = atoiOr(v, 5)
				return m.cfg.SeekStepSeconds, nil
			}},
		{group: "Playback", label: "Volume step", kind: setInt, key: "volume_step",
			min: 1, max: 25, step: 1, unit: " %",
			help:  "How much + and - change the volume.",
			value: func(m Model) string { return itoa(m.cfg.VolumeStep) },
			set:   func(m *Model, v string) (any, tea.Cmd) { m.cfg.VolumeStep = atoiOr(v, 5); return m.cfg.VolumeStep, nil }},

		// --- look
		{group: "Look", label: "Theme", kind: setChoice, key: "theme", options: themeNames(),
			help:  "Colour palette of the whole interface.",
			value: func(m Model) string { return m.themeName },
			set:   func(m *Model, v string) (any, tea.Cmd) { m.setTheme(v); return v, nil }},
		{group: "Look", label: "Accent from the cover", kind: setBool, section: "cover", key: "dynamic_accent",
			help:  "Take the accent colour from the album cover.",
			value: func(m Model) string { return boolStr(m.dynamicAccent) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.dynamicAccent = v == "true"
				m.applyAccent()
				return m.dynamicAccent, nil
			}},
		{group: "Look", label: "Now Playing layout", kind: setChoice, section: "now_playing", key: "layout",
			options: npLayouts,
			help:    "auto picks by the window: panels, split or compact.",
			value:   func(m Model) string { return m.npChoice },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.setNpLayout(v); return v, nil }},
		{group: "Look", label: "Progress bar", kind: setChoice, section: "ui", key: "progress_style",
			options: progressStyles,
			help:    "plain, a gradient of the cover's colour, or the gradient with a shimmer running along it.",
			value:   func(m Model) string { return m.progressStyle() },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.cfg.UI.ProgressStyle = v; return v, nil }},
		{group: "Look", label: "Start-up animation", kind: setBool, section: "ui", key: "splash",
			help:  "An equalizer dancing while spotui connects.",
			value: func(m Model) string { return boolStr(m.cfg.UI.Splash) },
			set:   func(m *Model, v string) (any, tea.Cmd) { m.cfg.UI.Splash = v == "true"; return m.cfg.UI.Splash, nil }},
		{group: "Look", label: "Easter eggs", kind: setBool, section: "ui", key: "easter_eggs",
			help:  "A few tracks bring a surprise of their own to the Windows Terminal shader.",
			value: func(m Model) string { return boolStr(m.cfg.UI.EasterEggs) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.UI.EasterEggs = v == "true"
				return m.cfg.UI.EasterEggs, nil
			}},
		{group: "Look", label: "Heart sparkle on like", kind: setBool, section: "ui", key: "like_animation",
			help:  "The heart sparkles for a moment when you like a track.",
			value: func(m Model) string { return boolStr(m.cfg.UI.LikeAnimation) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.UI.LikeAnimation = v == "true"
				return m.cfg.UI.LikeAnimation, nil
			}},
		{group: "Look", label: "Covers", kind: setBool, section: "cover", key: "enabled",
			help:  "Show album covers at all.",
			value: func(m Model) string { return boolStr(m.cfg.Cover.Enabled) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Cover.Enabled = v == "true"
				if m.img != nil {
					m.img.Invalidate()
				}
				return m.cfg.Cover.Enabled, nil
			}},
		{group: "Look", label: "Real images", kind: setBool,
			help:  "Draw covers as images (Sixel or kitty) rather than pixel blocks.",
			hide:  func(m Model) bool { return m.img == nil },
			value: func(m Model) string { return boolStr(m.imageMode) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.imageMode = v == "true"
				m.img.Invalidate()
				return nil, nil
			}},
		{group: "Look", label: "Image pixelation", kind: setChoice, section: "cover", key: "image_pixels",
			options: pixLabels, unit: " px",
			help:  "Pixels per side of a real cover image; 0 is full detail.",
			value: func(m Model) string { return itoa(m.imgPixels) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.imgPixels = atoiOr(v, 64)
				if m.img != nil {
					m.img.Invalidate()
				}
				return m.imgPixels, nil
			}},
		{group: "Look", label: "Pixel-block size", kind: setInt, section: "cover", key: "pixel_size",
			min: minPixelSize, max: maxPixelSize, step: 4, unit: " px",
			help:  "Resolution of covers drawn with blocks.",
			value: func(m Model) string { return itoa(m.pixSize) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.pixSize = clamp(atoiOr(v, 32), minPixelSize, maxPixelSize)
				m.repixelate()
				return m.pixSize, nil
			}},
		{group: "Look", label: "Image protocol", kind: setChoice, section: "cover", key: "mode", restart: true,
			options: []string{"auto", "sixel", "kitty", "blocks"},
			help:    "How real images reach the terminal; auto detects it. Needs a new start of spotui.",
			value:   func(m Model) string { return m.cfg.Cover.Mode },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.cfg.Cover.Mode = v; return v, nil }},

		// --- visualizer
		{group: "Visualizer", label: "Visualizer", kind: setBool, section: "visualizer", key: "enabled",
			help:  "The spectrum under Now Playing.",
			value: func(m Model) string { return boolStr(m.vizOn) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.vizOn = v == "true" && m.vizTap != nil
				return v == "true", nil
			}},
		{group: "Visualizer", label: "Style", kind: setChoice, section: "visualizer", key: "style",
			options: []string{"bars", "mirror", "wave"},
			help:    "Bars, mirrored bars, or an oscilloscope.",
			value:   func(m Model) string { return m.vizStyle },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.vizStyle = v; m.vizWaveGain = 1; return v, nil }},
		{group: "Visualizer", label: "Delay", kind: setChoice, section: "visualizer", key: "delay_ms",
			options: vizDelayOptions(),
			help:    "auto matches the audio buffer; raise it if the bars run ahead of what you hear.",
			value: func(m Model) string {
				if m.cfg.Visualizer.DelayMs < 0 {
					return "auto"
				}
				return itoa(m.cfg.Visualizer.DelayMs)
			},
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Visualizer.DelayMs = atoiOr(v, -1)
				if v == "auto" {
					m.cfg.Visualizer.DelayMs = -1
				}
				return m.cfg.Visualizer.DelayMs, nil
			}},
		{group: "Visualizer", label: "Bar width", kind: setInt, section: "visualizer", key: "bar_width",
			min: 1, max: 4, step: 1,
			help:  "Columns per bar.",
			value: func(m Model) string { return itoa(m.cfg.Visualizer.BarWidth) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Visualizer.BarWidth = atoiOr(v, 2)
				return m.cfg.Visualizer.BarWidth, nil
			}},

		// --- lyrics
		{group: "Lyrics", label: "Lyrics", kind: setBool, section: "lyrics", key: "enabled",
			help:  "Synced lyrics from LRCLIB.",
			value: func(m Model) string { return boolStr(m.lyr != nil) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				on := v == "true"
				m.cfg.Lyrics.Enabled = on
				var cmd tea.Cmd
				switch {
				case on && m.lyr == nil:
					m.lyr = newLyricsClient(m.cfg, m.eng)
					if m.track != nil {
						cmd = m.fetchLyrics(m.track)
					}
				case !on:
					m.lyr = nil
				}
				return on, cmd
			}},
		{group: "Lyrics", label: "Effects preset", kind: setChoice, section: "lyrics", key: "effects",
			options: fxLevels,
			help:    "Ready-made sets: subtle flashes on the beat, party adds sparkles and a shake on drops, nightcore adds the letter wave, the spread and flowing colours. custom uses the switches below; changing any of them switches to custom.",
			value:   func(m Model) string { return m.fxLevel() },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.cfg.Lyrics.Effects = v; return v, nil }},
		fxSwitch("Flash on the beat", "flash", "The line lights up in the cover's colour on each beat.",
			func(s fxSet) bool { return s.flash }, func(f *config.LyricsFXConfig, on bool) { f.Flash = on }),
		fxSwitch("Flowing colours", "colors", "The sung part flows through shades of the cover's colour in the track's tempo.",
			func(s fxSet) bool { return s.colors }, func(f *config.LyricsFXConfig, on bool) { f.Colors = on }),
		fxSwitch("Letter wave", "wave", "On each beat a wave runs through the line; the letter at its crest hops up.",
			func(s fxSet) bool { return s.wave }, func(f *config.LyricsFXConfig, on bool) { f.Wave = on }),
		fxSwitch("Letter spread", "spread", "On the hardest beats the letters spread apart for a moment.",
			func(s fxSet) bool { return s.spread }, func(f *config.LyricsFXConfig, on bool) { f.Spread = on }),
		fxSwitch("Sparkles", "sparkles", "Sparkles fly off the line on beats.",
			func(s fxSet) bool { return s.sparkles }, func(f *config.LyricsFXConfig, on bool) { f.Sparkles = on }),
		fxSwitch("Shake on drops", "shake", "The line shakes when the track drops, only then.",
			func(s fxSet) bool { return s.shake }, func(f *config.LyricsFXConfig, on bool) { f.Shake = on }),
		{group: "Lyrics", label: "Windows Terminal shader", kind: setBool, section: "lyrics", key: "wt_shader",
			help:  "Feeds the beat to shaders/spotui.hlsl, a Windows Terminal pixel shader that draws behind the lyrics. Set it as experimental.pixelShaderPath in your Windows Terminal profile first, or you will see coloured squares in the corner.",
			value: func(m Model) string { return boolStr(m.cfg.Lyrics.WTShader) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Lyrics.WTShader = v == "true"
				return m.cfg.Lyrics.WTShader, nil
			}},
		{group: "Lyrics", label: "Shader effect", kind: setChoice, section: "lyrics", key: "wt_shader_style",
			options: shaderChoices,
			help:    "What the Windows Terminal shader draws behind the lyrics. auto moves between the effects with the music. Switches at once, no reload.",
			value: func(m Model) string {
				if m.shaderIsAuto() {
					return shaderAuto
				}
				return shaderStyles[m.shaderSetStyle()]
			},
			set: func(m *Model, v string) (any, tea.Cmd) { m.cfg.Lyrics.WTShaderStyle = v; return v, nil }},
		{group: "Lyrics", label: "Shader motion", kind: setBool, section: "lyrics", key: "wt_shader_motion",
			help:  "The shader's view moves with the music, the way MilkDrop does: it zooms in with the bass, turns with the treble and ripples with the vocals and melody, each effect as suits it (the tunnel turns without zooming, the aurora only ripples).",
			value: func(m Model) string { return boolStr(m.cfg.Lyrics.WTShaderMotion) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Lyrics.WTShaderMotion = v == "true"
				return m.cfg.Lyrics.WTShaderMotion, nil
			}},
		{group: "Lyrics", label: "Shader quality", kind: setChoice, section: "lyrics", key: "wt_shader_quality",
			options: shaderQualities,
			help:    "How much the heavy shader effects draw (chains: how many chains and how finely). Windows Terminal runs the shader on every pixel, so on a laptop without a dedicated GPU pick low if the window stutters; high for a desktop GPU.",
			value:   func(m Model) string { return shaderQualities[m.shaderQuality()] },
			set:     func(m *Model, v string) (any, tea.Cmd) { m.cfg.Lyrics.WTShaderQuality = v; return v, nil }},
		{group: "Lyrics", label: "Line on classic Now Playing", kind: setBool, section: "lyrics", key: "now_playing_line",
			help:  "The current line under the progress bar in the classic layout.",
			value: func(m Model) string { return boolStr(m.cfg.Lyrics.NowPlayingLine) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Lyrics.NowPlayingLine = v == "true"
				return m.cfg.Lyrics.NowPlayingLine, nil
			}},

		// --- system
		{group: "System", label: "y copies", kind: setChoice, section: "ui", key: "copy_as",
			options: []string{"url", "uri"},
			help:    "url: an open.spotify.com link anyone can open; uri: spotify:track:… for apps.",
			value: func(m Model) string {
				if m.cfg.UI.CopyAs == "uri" {
					return "uri"
				}
				return "url"
			},
			set: func(m *Model, v string) (any, tea.Cmd) { m.cfg.UI.CopyAs = v; return v, nil }},
		{group: "System", label: "Notifications", kind: setBool, key: "notifications",
			help:  "A desktop notification when the track changes.",
			value: func(m Model) string { return boolStr(m.notifier != nil && !m.notifyOff) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				on := v == "true"
				m.cfg.Notifications = on
				m.notifyOff = !on
				if on && m.notifier == nil {
					m.notifier = notify.New(config.IsWSL())
				}
				return on, nil
			}},
		{group: "System", label: "Media keys", kind: setBool, key: "media_controls", restart: true,
			help:  "Media keys and system controls (MPRIS on Linux). Needs a new start of spotui.",
			value: func(m Model) string { return boolStr(m.cfg.MediaControls) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.MediaControls = v == "true"
				return m.cfg.MediaControls, nil
			}},
		{group: "System", label: "New playlists public", kind: setBool, section: "playlists", key: "public_by_default",
			help:  "Show playlists you make (N) on your profile.",
			value: func(m Model) string { return boolStr(m.cfg.Playlists.PublicByDefault) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Playlists.PublicByDefault = v == "true"
				return m.cfg.Playlists.PublicByDefault, nil
			}},
		{group: "System", label: "Undo time", kind: setInt, section: "playlists", key: "undo_seconds",
			min: 0, max: 60, step: 5, unit: " s",
			help:  "How long U can put back a track taken out of a playlist. 0 is off.",
			value: func(m Model) string { return itoa(m.cfg.Playlists.UndoSeconds) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Playlists.UndoSeconds = atoiOr(v, 10)
				return m.cfg.Playlists.UndoSeconds, nil
			}},
		{group: "System", label: "Remote control", kind: setBool, section: "control", key: "enabled", restart: true,
			help:  "spotui next, spotui play <link>, spotui now for status bars. Needs a new start of spotui.",
			value: func(m Model) string { return boolStr(m.cfg.Control.Enabled) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Control.Enabled = v == "true"
				return m.cfg.Control.Enabled, nil
			}},
		{group: "System", label: "Extra status file", kind: setText, section: "control", key: "status_file", restart: true,
			help:  "One more place for spotui now's file, e.g. the Windows side from WSL. Needs a new start of spotui.",
			value: func(m Model) string { return m.cfg.Control.StatusFile },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Control.StatusFile = strings.TrimSpace(v)
				return m.cfg.Control.StatusFile, nil
			}},
		{group: "System", label: "On track change", kind: setText, section: "hooks", key: "on_track_change",
			help:  "A shell command run on every new track, with SPOTUI_TITLE, SPOTUI_ARTIST, … set.",
			value: func(m Model) string { return m.cfg.Hooks.OnTrackChange },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Hooks.OnTrackChange = strings.TrimSpace(v)
				return m.cfg.Hooks.OnTrackChange, nil
			}},

		// --- updates
		{group: "Updates", label: "Release channel", kind: setChoice, section: "update", key: "prerelease",
			options: []string{"stable", "pre-release"},
			help:    "pre-release also offers releases marked pre-release on GitHub.",
			value: func(m Model) string {
				if m.cfg.Update.Prerelease {
					return "pre-release"
				}
				return "stable"
			},
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Update.Prerelease = v == "pre-release"
				return m.cfg.Update.Prerelease, nil
			}},
		{group: "Updates", label: "Check daily", kind: setBool, section: "update", key: "check",
			help:  "Look for a new release once a day and show it in the header.",
			value: func(m Model) string { return boolStr(m.cfg.Update.Check) },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.Update.Check = v == "true"
				return m.cfg.Update.Check, nil
			}},
		{group: "Updates", label: "Check now", kind: setAction,
			help: "Ask GitHub for the newest release on your channel.",
			run:  func(m *Model) tea.Cmd { m.setFlash("Checking for updates…", false); return m.checkUpdateNow() }},
		{group: "Updates", label: "Install update", kind: setAction,
			help:  "Download the new release and replace this binary; start spotui again afterwards.",
			hide:  func(m Model) bool { return m.updateTag == "" },
			value: func(m Model) string { return m.updateTag },
			run:   func(m *Model) tea.Cmd { return m.installUpdate() }},

		// --- account
		{group: "Account", label: "Spotify app client id", kind: setText, key: "client_id", restart: true,
			help:  "Your own app from developer.spotify.com, for search and stats. Empty uses a shared, rate-limited one.",
			value: func(m Model) string { return m.cfg.ClientID },
			set: func(m *Model, v string) (any, tea.Cmd) {
				m.cfg.ClientID = strings.TrimSpace(v)
				return m.cfg.ClientID, nil
			}},
		{group: "Account", label: "Restart player", kind: setAction,
			help: "Start the player again to apply the changes marked ↻.",
			run:  func(m *Model) tea.Cmd { m.restart = true; return tea.Quit }},
	}
	out := s[:0]
	for _, x := range s {
		if x.hide == nil || !x.hide(m) {
			out = append(out, x)
		}
	}
	_ = c
	return out
}

func (m *Model) openSettings() tea.Cmd {
	m.closeFilter()
	m.overlay = overlaySettings
	m.set.editing = false
	return nil
}

// changeSetting applies a new value and writes it to config.toml.
func (m *Model) changeSetting(s setting, v string) tea.Cmd {
	if s.set == nil {
		return nil
	}
	persist, cmd := s.set(m, v)
	if persist != nil && s.key != "" {
		if err := m.cfg.Set(s.section, s.key, persist); err != nil {
			m.setFlash("Could not save config.toml: "+err.Error(), true)
		}
	}
	if s.restart {
		m.set.restart = true
	}
	return cmd
}

// stepSetting moves a choice, number or switch one step in dir.
func (m *Model) stepSetting(s setting, dir int) tea.Cmd {
	cur := s.value(*m)
	switch s.kind {
	case setBool:
		return m.changeSetting(s, boolStr(cur != "true"))
	case setChoice:
		idx := 0
		for i, o := range s.options {
			if o == cur {
				idx = i
			}
		}
		idx = (idx + dir + len(s.options)) % len(s.options)
		return m.changeSetting(s, s.options[idx])
	case setInt:
		n := clamp(atoiOr(cur, s.min)+dir*s.step, s.min, s.max)
		return m.changeSetting(s, itoa(n))
	}
	return nil
}

func (m Model) updateSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	list := m.settings()
	if len(list) == 0 {
		m.overlay = overlayNone
		return m, nil
	}
	m.set.cursor = clamp(m.set.cursor, 0, len(list)-1)
	cur := list[m.set.cursor]
	key := msg.String()

	if m.set.editing {
		switch key {
		case "enter":
			m.set.editing = false
			return m, m.changeSetting(cur, m.set.input.Value())
		case "esc":
			m.set.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.set.input, cmd = m.set.input.Update(msg)
		return m, cmd
	}

	switch key {
	case "esc", ",", "q":
		m.overlay = overlayNone
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.set.cursor = max(m.set.cursor-1, 0)
	case "down", "j":
		m.set.cursor = min(m.set.cursor+1, len(list)-1)
	case "home", "g":
		m.set.cursor = 0
	case "end", "G":
		m.set.cursor = len(list) - 1
	case "left", "h":
		return m, m.stepSetting(cur, -1)
	case "right", "l":
		return m, m.stepSetting(cur, +1)
	case "enter", " ":
		switch cur.kind {
		case setAction:
			return m, cur.run(&m)
		case setText:
			m.set.input = textinput.New()
			m.set.input.Prompt = ""
			m.set.input.CharLimit = 200
			m.set.input.SetValue(cur.value(m))
			m.set.input.CursorEnd()
			m.set.editing = true
			return m, m.set.input.Focus()
		default:
			return m, m.stepSetting(cur, +1)
		}
	case "R":
		if m.set.restart {
			m.restart = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewSettings(w, h int) string {
	cw := clamp(w-8, 50, 96)
	list := m.settings()
	cursor := clamp(m.set.cursor, 0, max(len(list)-1, 0))
	labelW := 28
	valueW := cw - labelW - 10

	type row struct {
		text string
		idx  int // setting index, -1 for a group heading
	}
	var rows []row
	group := ""
	for i, s := range list {
		if s.group != group {
			if group != "" {
				rows = append(rows, row{"", -1})
			}
			group = s.group
			rows = append(rows, row{m.theme.accentTxt.Bold(true).Render(strings.ToUpper(group)), -1})
		}
		rows = append(rows, row{m.settingRow(s, i == cursor, labelW, valueW), i})
	}

	// Keep the cursor's row in view.
	listH := clamp(h-12, 6, len(rows))
	at := 0
	for i, r := range rows {
		if r.idx == cursor {
			at = i
		}
	}
	start := clamp(at-listH/2, 0, max(len(rows)-listH, 0))
	var lines []string
	for _, r := range rows[start:min(start+listH, len(rows))] {
		lines = append(lines, r.text)
	}

	title := m.theme.title.Render("Settings") + m.theme.dim.Render("   saved to config.toml as you change them")
	help := ""
	if len(list) > 0 {
		help = list[cursor].help
		if list[cursor].restart {
			help += " " + m.theme.accentTxt.Render("↻ after a restart")
		}
	}
	keys := "↑↓ choose · ←→ change · enter toggle / edit · esc close"
	if m.set.editing {
		keys = "type · enter save · esc cancel"
	}
	parts := []string{title, "", strings.Join(lines, "\n"), "",
		lipgloss.NewStyle().Width(cw - 6).Foreground(colSubtle).Render(help)}
	if m.set.restart {
		parts = append(parts, "", m.theme.accentTxt.Render("Some changes wait for a restart · R restarts the player now"))
	}
	parts = append(parts, "", m.theme.dim.Render(keys))
	return m.theme.card.Padding(1, 2).Width(cw).Render(strings.Join(parts, "\n"))
}

func (m Model) settingRow(s setting, selected bool, labelW, valueW int) string {
	label := ansi.Truncate(s.label, labelW-2, "…")
	label += strings.Repeat(" ", max(labelW-ansi.StringWidth(label), 0))

	var value string
	cur := ""
	if s.value != nil {
		cur = s.value(m)
	}
	switch s.kind {
	case setBool:
		if cur == "true" {
			value = m.theme.accentTxt.Render("● on")
		} else {
			value = m.theme.dim.Render("○ off")
		}
	case setChoice, setInt:
		shown := cur + s.unit
		if s.kind == setInt && cur == "0" && s.key == "crossfade_ms" {
			shown = "off"
		}
		if s.key == "delay_ms" && cur == "auto" {
			shown = "auto (" + itoa(m.vizDelayMs()) + " ms)"
		}
		if s.key == "image_pixels" && cur == "0" {
			shown = "full detail"
		}
		value = m.theme.dim.Render("‹ ") + m.theme.text.Render(shown) + m.theme.dim.Render(" ›")
	case setText:
		if selected && m.set.editing {
			in := m.set.input
			in.Width = valueW - 2
			value = in.View()
		} else if cur == "" {
			value = m.theme.dim.Render("(empty)")
		} else {
			value = m.theme.text.Render(cur)
		}
	case setAction:
		value = m.theme.accentTxt.Render("↵ " + strings.ToLower(s.label))
		if cur != "" {
			value += m.theme.dim.Render("  " + cur)
		}
	}
	if s.restart {
		value += m.theme.dim.Render("  ↻")
	}
	value = ansi.Truncate(value, valueW, "…")

	if selected {
		return m.theme.accentTxt.Render("› ") + m.theme.title.Render(label) + value
	}
	return "  " + m.theme.text.Render(label) + value
}

// --- updates from the settings page

type updateCheckedMsg struct {
	tag, url string
	pre      bool
	err      error
}

type updateInstalledMsg struct {
	tag string
	err error
}

// checkUpdateNow asks GitHub (conditionally, with the cached ETag).
func (m Model) checkUpdateNow() tea.Cmd {
	dir, pre := m.cfg.CacheDir, m.cfg.Update.Prerelease
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rels, err := update.NewChecker(dir, Version).Releases(ctx, true)
		if err != nil {
			return updateCheckedMsg{err: err}
		}
		r := update.Pick(rels, pre)
		if r == nil || !update.Newer(r.Tag, Version) {
			return updateCheckedMsg{}
		}
		return updateCheckedMsg{tag: r.Tag, url: r.URL, pre: r.Prerelease}
	}
}

func (m *Model) installUpdate() tea.Cmd {
	if m.updateTag == "" {
		return nil
	}
	tag, dir, pre := m.updateTag, m.cfg.CacheDir, m.cfg.Update.Prerelease
	m.setFlash("Installing spotui "+tag+"…", false)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		rels, err := update.NewChecker(dir, Version).Releases(ctx, false)
		if err != nil {
			return updateInstalledMsg{tag: tag, err: err}
		}
		r := update.Pick(rels, pre)
		if r == nil || r.Tag != tag {
			return updateInstalledMsg{tag: tag, err: errString("the release is no longer offered")}
		}
		exe, err := update.Executable()
		if err != nil {
			return updateInstalledMsg{tag: tag, err: err}
		}
		return updateInstalledMsg{tag: tag, err: update.Install(ctx, nil, r, exe, Version)}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func (m *Model) handleUpdateResult(msg tea.Msg) {
	switch msg := msg.(type) {
	case updateCheckedMsg:
		switch {
		case msg.err != nil:
			m.setFlash("Update check failed: "+msg.err.Error(), true)
		case msg.tag == "":
			m.setFlash("spotui "+Version+" is the newest on your channel", false)
		default:
			m.updateTag = msg.tag
			m.setFlash("spotui "+msg.tag+" is out · Install update in Settings, or run `spotui update`", false)
		}
	case updateInstalledMsg:
		if msg.err != nil {
			m.setFlash("Update failed: "+msg.err.Error(), true)
			return
		}
		m.updateTag = ""
		m.setFlash("Installed spotui "+msg.tag+" · quit and start spotui again to use it", false)
	}
}

// fxSwitch is one lyric effect's switch. Turning one while a preset is in
// use copies the preset's set into [lyrics.fx] first and moves to custom,
// so only that one effect changes.
func fxSwitch(label, key, help string, get func(fxSet) bool, put func(*config.LyricsFXConfig, bool)) setting {
	return setting{group: "Lyrics", label: "  " + label, kind: setBool, section: "lyrics.fx", key: key, help: help,
		value: func(m Model) string { return boolStr(get(m.fxOn())) },
		set: func(m *Model, v string) (any, tea.Cmd) {
			if m.fxLevel() != fxCustom {
				cur := m.fxOn()
				m.cfg.Lyrics.FX = config.LyricsFXConfig{Flash: cur.flash, Colors: cur.colors, Wave: cur.wave,
					Spread: cur.spread, Sparkles: cur.sparkles, Shake: cur.shake}
				m.cfg.Lyrics.Effects = fxCustom
				f := m.cfg.Lyrics.FX
				for k, on := range map[string]bool{"flash": f.Flash, "colors": f.Colors, "wave": f.Wave,
					"spread": f.Spread, "sparkles": f.Sparkles, "shake": f.Shake} {
					_ = m.cfg.Set("lyrics.fx", k, on)
				}
				_ = m.cfg.Set("lyrics", "effects", fxCustom)
			}
			on := v == "true"
			put(&m.cfg.Lyrics.FX, on)
			return on, nil
		}}
}
