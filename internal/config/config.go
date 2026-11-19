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
	// The default keys keep working. See the README for action names.
	Keys map[string]string `toml:"keys"`

	// Lyrics controls synced lyrics.
	Lyrics LyricsConfig `toml:"lyrics"`

	// Notifications shows a desktop notification when the track changes.
	Notifications bool `toml:"notifications"`
	// MediaControls exposes the player to the OS: MPRIS on Linux (media
	// keys, playerctl, desktop widgets) and global media keys on Windows.
	MediaControls bool `toml:"media_controls"`

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
	// CacheSizeMB caps the on-disk cover cache.
	CacheSizeMB int `toml:"cache_size_mb"`
}

// VisualizerConfig controls the spectrum analyzer on Now Playing.
type VisualizerConfig struct {
	Enabled bool `toml:"enabled"`
	// DelayMs shifts the analysed audio back to match what is audible; the
	// output reads ahead of playback by its buffer size.
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
		},
		Visualizer: VisualizerConfig{
			Enabled:  true,
			DelayMs:  120,
			BarWidth: 2,
			Style:    "bars",
		},
		Lyrics: LyricsConfig{
			Enabled:        true,
			NowPlayingLine: true,
		},
		SeekStepSeconds: 5,
		VolumeStep:      5,
		Notifications:   true,
		MediaControls:   true,
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
