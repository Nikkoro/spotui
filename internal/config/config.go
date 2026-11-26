// Package config loads and stores the spotui configuration file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

const appName = "spotui"

// Config is the user-editable configuration stored in config.toml.
type Config struct {
	// DeviceName is the name shown in the Spotify Connect device list.
	DeviceName string `toml:"device_name"`

	// ClientID is your own Spotify app's client id, used for Web API calls
	// such as search. Register http://127.0.0.1:8989/login as a redirect URI.
	// Empty falls back to the session token, which is heavily rate limited.
	ClientID string `toml:"client_id"`

	// AudioBackend selects the output driver: alsa, pulseaudio, wasapi,
	// audio-toolbox or pipe. Empty picks the best one for the platform.
	AudioBackend string `toml:"audio_backend"`
	// AudioDevice is the output device name (alsa and pulseaudio only).
	AudioDevice string `toml:"audio_device"`

	// Bitrate is the preferred stream quality: 96, 160 or 320.
	Bitrate int `toml:"bitrate"`
	// InitialVolume is the volume (0-100) used when no previous volume is known.
	InitialVolume uint32 `toml:"initial_volume"`
	// Normalisation enables Spotify loudness normalisation.
	Normalisation bool `toml:"normalisation"`
	// CrossfadeMs blends the end of a track into the next one (0 is off).
	CrossfadeMs int `toml:"crossfade_ms"`
	// Autoplay keeps similar music going when a playlist or album ends.
	Autoplay bool `toml:"autoplay"`

	// AudioCache keeps downloaded (encrypted) audio on disk.
	AudioCache bool `toml:"audio_cache"`
	// AudioCacheSizeMB caps the audio cache size; 0 means unlimited.
	AudioCacheSizeMB int64 `toml:"audio_cache_size_mb"`

	// Theme is the colour palette: spotify, catppuccin, gruvbox, nord,
	// rose-pine or tokyo-night.
	Theme string `toml:"theme"`

	// Colors overrides single colours of the theme (#rrggbb): accent, text,
	// subtle, dim, track, selection, background_text.
	Colors map[string]string `toml:"colors"`

	// Cover controls album art rendering.
	Cover CoverConfig `toml:"cover"`

	// Visualizer controls the spectrum analyzer.
	Visualizer VisualizerConfig `toml:"visualizer"`

	// SeekStepSeconds is how far ← / → seek.
	SeekStepSeconds int `toml:"seek_step_seconds"`
	// VolumeStep is how much + / - change the volume, in percent.
	VolumeStep int `toml:"volume_step"`

	// Keys adds key bindings: action = "key" (several separated by commas).
	// The default keys keep working. Action names: see [keys] in the README.
	Keys map[string]string `toml:"keys"`

	// Lyrics controls synced lyrics.
	Lyrics LyricsConfig `toml:"lyrics"`

	// NowPlaying controls the Now Playing page.
	NowPlaying NowPlayingConfig `toml:"now_playing"`

	// Update controls checking for and installing new releases.
	Update UpdateConfig `toml:"update"`

	// UI holds small touches of the interface.
	UI UIConfig `toml:"ui"`

	// Notifications shows a desktop notification when the track changes.
	Notifications bool `toml:"notifications"`
	// MediaControls exposes the player to the OS: MPRIS on Linux (media
	// keys, playerctl, desktop widgets) and global media keys on Windows.
	MediaControls bool `toml:"media_controls"`

	// Control lets other programs steer and read the running spotui.
	Control ControlConfig `toml:"control"`
	// Hooks run commands of your own on player events.
	Hooks HooksConfig `toml:"hooks"`

	// Dirs are resolved at load time and never written to the file.
	ConfigDir string `toml:"-"`
	CacheDir  string `toml:"-"`
	// Warnings lists keys in the file that mean nothing, usually typos.
	Warnings []string `toml:"-"`
}

// CoverConfig controls how the album cover is rendered.
type CoverConfig struct {
	// Enabled shows the cover at all.
	Enabled bool `toml:"enabled"`
	// PixelSize is the resolution of the pixel-art cover in pixels per side.
	PixelSize int `toml:"pixel_size"`
	// Levels is the number of colour levels per channel (posterisation).
	// 0 or 256 keeps the full palette.
	Levels int `toml:"levels"`
	// DynamicAccent derives the UI accent colour from the cover.
	DynamicAccent bool `toml:"dynamic_accent"`
	// Mode is how covers are drawn: "auto" (real images where the terminal
	// supports Sixel or the kitty protocol, pixel blocks elsewhere),
	// "sixel", "kitty" or "blocks".
	Mode string `toml:"mode"`
	// ImagePixels pixelates real images to this many pixels per side
	// (0: full detail).
	ImagePixels int `toml:"image_pixels"`
	// Size caps the cover width in columns (0: as large as fits).
	Size int `toml:"size"`
	// CacheSizeMB caps the on-disk cover cache.
	CacheSizeMB int `toml:"cache_size_mb"`
}

// VisualizerConfig controls the spectrum analyzer on Now Playing.
type VisualizerConfig struct {
	Enabled bool `toml:"enabled"`
	// DelayMs shifts the analysed audio back to match what is audible; the
	// output reads ahead of playback by its buffer size. -1 matches it to
	// the audio output's buffer.
	DelayMs int `toml:"delay_ms"`
	// BarWidth is the width of each bar in columns.
	BarWidth int `toml:"bar_width"`
	// Style is "bars", "mirror" or "wave".
	Style string `toml:"style"`
}

// LyricsConfig controls synced lyrics from LRCLIB.
type LyricsConfig struct {
	Enabled bool `toml:"enabled"`
	// NowPlayingLine shows the current lyric line on Now Playing.
	NowPlayingLine bool `toml:"now_playing_line"`
	// Effects animates the line being sung to the beat. Presets: "off",
	// "subtle" (it flashes), "party" (sparkles, and a shake when the track
	// drops), "nightcore" (also letter waves, a letter spread on hard
	// beats and flowing colours). "custom" uses the switches in FX.
	Effects string `toml:"effects"`
	// FX holds each effect's own switch, used when Effects is "custom".
	FX LyricsFXConfig `toml:"fx"`
	// WTShader paints a few cells in the top-left corner that carry the
	// music's beat and level to a Windows Terminal pixel shader
	// (shaders/spotui.hlsl), which draws behind the lyrics. Off by
	// default: without the shader the cells show as coloured squares.
	WTShader bool `toml:"wt_shader"`
	// WTShaderStyle is the shader's effect: "plasma", "tunnel", "warp",
	// "aurora", "synthwave", "chains", "rain" or "auto"
	// ("debug" shows how evenly frames arrive).
	WTShaderStyle string `toml:"wt_shader_style"`
	// WTShaderMotion moves the shader's camera with the music, the way
	// MilkDrop does: zooming with the bass, turning with the treble,
	// rippling with the mid.
	WTShaderMotion bool `toml:"wt_shader_motion"`
	// WTShaderQuality is how much the heavy effects (chains) draw:
	// "low", "medium" or "high". Low for a laptop without a dedicated GPU.
	WTShaderQuality string `toml:"wt_shader_quality"`
}

type NowPlayingConfig struct {
	// Layout is "auto", "split", "panels", "compact" or "classic". L
	// switches it while running, and that choice is remembered.
	Layout string `toml:"layout"`
}

// LyricsFXConfig switches each lyric beat effect on its own ([lyrics.fx]).
type LyricsFXConfig struct {
	Flash    bool `toml:"flash"`    // the line lights up in the accent on each beat
	Colors   bool `toml:"colors"`   // the sung part flows through colours in tempo
	Wave     bool `toml:"wave"`     // a wave runs through the line letter by letter
	Spread   bool `toml:"spread"`   // the letters spread apart on hard beats
	Sparkles bool `toml:"sparkles"` // sparkles fly off the line on beats
	Shake    bool `toml:"shake"`    // the line shakes when the track drops
}

type UIConfig struct {
	// ProgressStyle is "plain", "gradient" (the cover's colour, light to
	// dark) or "shimmer" (gradient with a light running along it).
	ProgressStyle string `toml:"progress_style"`
	// Splash animates the start-up screen.
	Splash bool `toml:"splash"`
	// LikeAnimation sparkles the heart when you like a track.
	LikeAnimation bool `toml:"like_animation"`
	// CopyAs is what y copies: "url" (open.spotify.com link) or "uri".
	CopyAs string `toml:"copy_as"`
	// EasterEggs lets a few tracks bring a surprise of their own to the
	// Windows Terminal shader.
	EasterEggs bool `toml:"easter_eggs"`
}

// ControlConfig is [control]: `spotui next`, `spotui now` and the like.
type ControlConfig struct {
	// Enabled listens for commands from other terminals (`spotui next`,
	// `spotui play <link>`, ...) on a local socket and keeps the state file
	// `spotui now` reads.
	Enabled bool `toml:"enabled"`
	// StatusFile is one more place to keep the state file, for example the
	// Windows side from WSL (/mnt/c/Users/you/AppData/Local/spotui/now.json),
	// where the Windows spotui.exe now reads it.
	StatusFile string `toml:"status_file"`
}

// HooksConfig is [hooks]: shell commands run in the background, with the
// track in SPOTUI_* variables.
type HooksConfig struct {
	// OnTrackChange runs when a new track starts.
	OnTrackChange string `toml:"on_track_change"`
}

type UpdateConfig struct {
	// Check looks for a newer release once a day and says so in the UI.
	Check bool `toml:"check"`
	// Prerelease includes releases marked pre-release on GitHub, for both
	// the check and `spotui update` (which also takes --pre).
	Prerelease bool `toml:"prerelease"`
}

// Default returns the configuration used when no file exists yet.
func Default() Config {
	host, _ := os.Hostname()
	name := appName
	if host != "" {
		name = fmt.Sprintf("%s (%s)", appName, host)
	}

	return Config{
		DeviceName:       name,
		Theme:            "spotify",
		Bitrate:          320,
		InitialVolume:    70,
		Normalisation:    true,
		Autoplay:         true,
		AudioCache:       true,
		AudioCacheSizeMB: 1024,
		Cover: CoverConfig{
			Enabled:       true,
			PixelSize:     32,
			Levels:        8,
			DynamicAccent: true,
			CacheSizeMB:   200,
			Mode:          "auto",
			ImagePixels:   64,
		},
		Visualizer: VisualizerConfig{
			Enabled:  true,
			DelayMs:  -1,
			BarWidth: 2,
			Style:    "bars",
		},
		Lyrics: LyricsConfig{
			Enabled:         true,
			NowPlayingLine:  true,
			Effects:         "subtle",
			WTShaderStyle:   "plasma",
			WTShaderMotion:  true,
			WTShaderQuality: "medium",
		},
		NowPlaying:      NowPlayingConfig{Layout: "auto"},
		Update:          UpdateConfig{Check: true},
		UI:              UIConfig{ProgressStyle: "shimmer", Splash: true, LikeAnimation: true, CopyAs: "url", EasterEggs: true},
		SeekStepSeconds: 5,
		VolumeStep:      5,
		Notifications:   true,
		MediaControls:   true,
		Control:         ControlConfig{Enabled: true},
	}
}

// Load reads config.toml from the config directory, creating it with
// defaults when it does not exist. dir overrides the default location.
func Load(dir string) (*Config, error) {
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("resolving config dir: %w", err)
		}
		dir = filepath.Join(base, appName)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	cfg := Default()
	path := filepath.Join(dir, "config.toml")

	switch meta, err := toml.DecodeFile(path, &cfg); {
	case err == nil:
		for _, k := range meta.Undecoded() {
			// [keys] and [colors] are free-form maps, checked elsewhere.
			if len(k) > 0 && (k[0] == "keys" || k[0] == "colors") {
				continue
			}
			cfg.Warnings = append(cfg.Warnings, "unknown setting "+k.String())
		}
	case errors.Is(err, os.ErrNotExist):
		if err := write(path, &cfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg.ConfigDir = dir
	if base, err := os.UserCacheDir(); err == nil {
		cfg.CacheDir = filepath.Join(base, appName)
	} else {
		cfg.CacheDir = filepath.Join(dir, "cache")
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	if cfg.AudioBackend == "" {
		cfg.AudioBackend = DefaultAudioBackend()
	}
	if cfg.AudioDevice == "" {
		cfg.AudioDevice = "default"
	}

	return &cfg, nil
}

func write(path string, cfg *Config) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString("# spotui configuration\n# Empty audio_backend picks the best driver for this platform.\n\n"); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(cfg)
}

// DefaultAudioBackend picks the output driver for the current platform.
// On Linux PulseAudio (or PipeWire's Pulse server) is preferred when a server
// is reachable, which also covers WSLg; plain ALSA is the fallback.
func DefaultAudioBackend() string {
	switch runtime.GOOS {
	case "windows":
		return "wasapi"
	case "darwin":
		return "audio-toolbox"
	}

	if os.Getenv("PULSE_SERVER") != "" {
		return "pulseaudio"
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "pulse", "native")); err == nil {
			return "pulseaudio"
		}
	}
	return "alsa"
}

// IsWSL reports whether the process runs inside Windows Subsystem for Linux.
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(b)), "microsoft")
}
