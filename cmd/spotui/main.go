// Command spotui is a terminal Spotify client that is itself a Spotify
// Connect device.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/output"
	"github.com/sirupsen/logrus"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/engine"
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
	flag.Parse()

	if *showVersion {
		fmt.Println("spotui", version)
		return nil
	}

	cfg, err := config.Load(*configDir)
	if err != nil {
		return err
	}

	// The TUI owns the terminal, so logs go to a file.
	logPath := filepath.Join(cfg.ConfigDir, "spotui.log")
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

	eng, err := engine.New(cfg, engine.NewLogger(logger))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		eng.Run(ctx)
	}()

	p := tea.NewProgram(ui.New(eng, cfg, tap), tea.WithAltScreen())
	_, uiErr := p.Run()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.Warn("player did not stop in time")
	}
	if err := eng.Close(); err != nil {
		logger.WithError(err).Warn("closing player")
	}
	return uiErr
}
