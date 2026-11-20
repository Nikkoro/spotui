// Command spotui is a terminal Spotify client that is itself a Spotify
// Connect device.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/output"
	"github.com/sirupsen/logrus"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/mediakeys"
	"github.com/Nikkoro/spotui/internal/termimg"
	"github.com/Nikkoro/spotui/internal/ui"
	"github.com/Nikkoro/spotui/internal/viz"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "spotui:", err)
		os.Exit(1)
	}
}

func run() error {
	configDir := flag.String("config-dir", "", "configuration directory (default: OS config dir/spotui)")
	debug := flag.Bool("debug", false, "verbose logging to spotui.log")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "spotui %s: a Spotify Connect player for the terminal\n\n", version)
		fmt.Fprintf(out, "usage: spotui [flags] [command]\n\ncommands:\n")
		fmt.Fprintf(out, "  (none)       start the player\n")
		fmt.Fprintf(out, "  logout       forget the Spotify login; the next start pairs again\n")
		fmt.Fprintf(out, "  paths        print where config, cache and logs live\n")
		fmt.Fprintf(out, "  clear-cache  delete cached covers, lyrics, library and audio\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("spotui", version)
		return nil
	}

	cfg, err := config.Load(*configDir)
	if err != nil {
		return err
	}

	switch cmd := flag.Arg(0); cmd {
	case "":
	case "logout":
		if err := engine.Logout(cfg.ConfigDir); err != nil {
			return err
		}
		fmt.Println("Logged out. The next start will show a pairing code.")
		return nil
	case "paths":
		fmt.Println("config:", filepath.Join(cfg.ConfigDir, "config.toml"))
		fmt.Println("state: ", filepath.Join(cfg.ConfigDir, "state.json"))
		fmt.Println("log:   ", filepath.Join(cfg.ConfigDir, "spotui.log"))
		fmt.Println("cache: ", cfg.CacheDir)
		return nil
	case "clear-cache":
		if err := os.RemoveAll(cfg.CacheDir); err != nil {
			return err
		}
		fmt.Println("Cleared", cfg.CacheDir)
		return nil
	default:
		flag.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}

	// The TUI owns the terminal, so logs go to a file. The previous run's
	// log is kept as spotui.log.1, handy after a crash or a restart.
	logPath := filepath.Join(cfg.ConfigDir, "spotui.log")
	_ = os.Rename(logPath, logPath+".1")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()

	logger := logrus.New()
	logger.SetOutput(logFile)
	logger.SetLevel(logrus.InfoLevel)
	if *debug {
		logger.SetLevel(logrus.DebugLevel)
	}
	logger.Infof("spotui %s, audio backend %s", version, cfg.AudioBackend)

	// Tap the PCM going to the audio device for the visualizer. This must be
	// set before the first output is opened.
	tap := viz.NewTap()
	output.ReaderHook = tap.Wrap

	// Config mistakes are logged and shown once in the interface.
	warnings := append([]string(nil), cfg.Warnings...)
	for _, w := range cfg.Warnings {
		logger.Warn("config.toml: " + w)
	}
	if _, unknown := ui.CheckKeys(cfg.Keys); len(unknown) > 0 {
		logger.Warnf("unknown actions in [keys]: %v (valid: %v)", unknown, ui.ActionNames())
		warnings = append(warnings, fmt.Sprintf("unknown [keys] actions %v", unknown))
	}
	if bad := ui.SetColorOverrides(cfg.Colors); len(bad) > 0 {
		logger.Warnf("ignored [colors] entries: %v (use accent, text, subtle, dim, track, selection, background_text = \"#rrggbb\")", bad)
		warnings = append(warnings, fmt.Sprintf("ignored [colors] %v", bad))
	}
	if !ui.ApplyPalette(cfg.Theme) {
		logger.Warnf("unknown theme %q, using the default", cfg.Theme)
		warnings = append(warnings, fmt.Sprintf("unknown theme %q", cfg.Theme))
	}
	ui.Version = version

	// Real cover images: ask the terminal before the interface takes it.
	caps := termimg.Detect(cfg.Cover.Mode)
	logger.Infof("covers: %s, cells %dx%d px", caps.Protocol, caps.CellW, caps.CellH)
	var imgOut *termimg.Writer
	if caps.Protocol != termimg.Blocks {
		imgOut = termimg.NewWriter(os.Stdout, caps.Protocol)
	}

	// Media keys outlive player restarts, so they talk to whichever engine
	// is current.
	var current atomic.Pointer[engine.Engine]
	if cfg.MediaControls {
		stopKeys, err := mediakeys.Listen(func(k mediakeys.Key) {
			eng := current.Load()
			if eng == nil {
				return
			}
			kctx, kcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer kcancel()
			switch k {
			case mediakeys.PlayPause:
				_ = eng.PlayPause(kctx)
			case mediakeys.Next:
				_ = eng.Next(kctx)
			case mediakeys.Prev:
				_ = eng.Prev(kctx)
			case mediakeys.Stop:
				_ = eng.Pause(kctx)
			}
		})
		if err == nil {
			defer stopKeys()
		} else if !errors.Is(err, errors.ErrUnsupported) {
			logger.WithError(err).Warn("media keys unavailable")
		}
	}

	// The player is restarted after it stops (lost session, network gone),
	// with the interface coming back where it was.
	for attempt := 0; ; attempt++ {
		started := time.Now()
		restart, err := runSession(cfg, logger, tap, &current, attempt, warnings, imgOut, caps)
		warnings = nil // shown once
		if err != nil || !restart {
			return err
		}
		if time.Since(started) > 2*time.Minute {
			attempt = 0 // it ran fine for a while: back off from scratch
		}
		logger.Infof("restarting the player (attempt %d)", attempt+1)
	}
}

// runSession runs one player and interface until the user quits (false) or
// the interface asks for a restart (true).
func runSession(cfg *config.Config, logger *logrus.Logger, tap *viz.Tap, current *atomic.Pointer[engine.Engine],
	attempt int, warnings []string, imgOut *termimg.Writer, caps termimg.Caps) (bool, error) {
	eng, err := engine.New(cfg, engine.NewLogger(logger))
	if err != nil {
		return false, err
	}
	current.Store(eng)
	defer current.Store(nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		eng.Run(ctx)
	}()

	// Mouse reporting starts once logged in, so the login link can still be
	// selected and copied with the mouse.
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if imgOut != nil {
		// Frames pass through the image writer, which draws the cover
		// after each frame that needs it.
		opts = append(opts, tea.WithOutput(imgOut))
	}
	model := ui.New(eng, cfg, tap).WithRestartAttempt(attempt).WithWarnings(warnings).WithImages(imgOut, caps)
	p := tea.NewProgram(model, opts...)
	final, uiErr := p.Run()
	restart := false
	if fm, ok := final.(ui.Model); ok {
		if err := fm.SaveState(); err != nil {
			logger.WithError(err).Warn("saving UI state")
		}
		restart = fm.WantsRestart()
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.Warn("player did not stop in time")
	}
	if err := eng.Close(); err != nil {
		logger.WithError(err).Warn("closing player")
	}
	return restart && uiErr == nil, uiErr
}
