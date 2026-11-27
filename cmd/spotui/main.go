// Command spotui is a terminal Spotify client that is itself a Spotify
// Connect device.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/output"
	"github.com/sirupsen/logrus"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/mediakeys"
	"github.com/Nikkoro/spotui/internal/remote"
	"github.com/Nikkoro/spotui/internal/termimg"
	"github.com/Nikkoro/spotui/internal/ui"
	"github.com/Nikkoro/spotui/internal/update"
	"github.com/Nikkoro/spotui/internal/viz"
	"github.com/Nikkoro/spotui/shaders"
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
		fmt.Fprintf(out, "  clear-cache  delete cached covers, lyrics, library and audio\n")
		fmt.Fprintf(out, "  shader       write the Windows Terminal shader (to a path, or next to config.toml)\n")
		fmt.Fprintf(out, "  update       install the newest release (--pre: pre-releases too, --check: only look)\n")
		fmt.Fprintf(out, "  now          print what the running spotui plays (--format, --json, --json=waybar)\n")
		fmt.Fprintf(out, "  play [link]  resume, or play a spotify: URI or open.spotify.com link\n")
		fmt.Fprintf(out, "  radio [link] radio of the playing song, or of a song, artist, album or playlist\n")
		fmt.Fprintf(out, "  pause, toggle, next, prev, like, unlike\n")
		fmt.Fprintf(out, "               steer the running spotui\n")
		fmt.Fprintf(out, "  volume N     set the volume to N %%, or change it by +N / -N\n\nflags:\n")
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
		fmt.Println("socket:", socketPath(cfg))
		for _, p := range statePaths(cfg) {
			fmt.Println("now:   ", p)
		}
		return nil
	case "now":
		return runNow(cfg, flag.Args()[1:])
	case "update":
		return runUpdate(cfg, flag.Args()[1:])
	case "shader":
		return runShader(cfg, flag.Arg(1))
	case "clear-cache":
		if err := os.RemoveAll(cfg.CacheDir); err != nil {
			return err
		}
		fmt.Println("Cleared", cfg.CacheDir)
		return nil
	default:
		if slices.Contains(ui.RemoteCommands, cmd) || cmd == "previous" {
			return runRemote(cfg, cmd, flag.Args()[1:])
		}
		flag.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}

	if exe, err := update.Executable(); err == nil {
		update.CleanupOld(exe) // what a Windows update moved aside
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

	// A shader written by an older spotui may read the cells differently.
	if wrote, err := shaders.Refresh(defaultShaderPath(cfg)); err != nil {
		logger.Warnf("updating the shader: %v", err)
	} else if wrote {
		logger.Info("shader brought up to date; open a new Windows Terminal tab to load it")
	}

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

	// Other terminals steer spotui through a socket and read what it plays
	// from a small file. Both outlive player restarts.
	var prog atomic.Pointer[tea.Program]
	var publisher *remote.Publisher
	if cfg.Control.Enabled {
		stop, err := remote.Listen(socketPath(cfg), remoteHandler(&prog), logger.Warnf)
		switch {
		case err == nil:
			defer stop()
			publisher = remote.NewPublisher(statePaths(cfg), logger.Warnf)
			defer publisher.Close()
		case errors.Is(err, remote.ErrRunning):
			// The other one keeps the state file; this one stays out of it.
			logger.Warn("control socket: another spotui is running and keeps it")
			warnings = append(warnings, "another spotui is running; `spotui next` and `spotui now` talk to that one")
		default:
			logger.WithError(err).Warn("control socket unavailable")
		}
	}

	// The player is restarted after it stops (lost session, network gone),
	// with the interface coming back where it was.
	for attempt := 0; ; attempt++ {
		started := time.Now()
		restart, err := runSession(cfg, logger, tap, &current, &prog, publisher, attempt, warnings, imgOut, caps)
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
	prog *atomic.Pointer[tea.Program], publisher *remote.Publisher, attempt int, warnings []string, imgOut *termimg.Writer, caps termimg.Caps) (bool, error) {
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
	model := ui.New(eng, cfg, tap).WithRestartAttempt(attempt).WithWarnings(warnings).WithImages(imgOut, caps).WithRemote(publisher)
	p := tea.NewProgram(model, opts...)
	prog.Store(p)
	final, uiErr := p.Run()
	prog.Store(nil)
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

// runUpdate installs the newest release over the running binary.
func runUpdate(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	pre := fs.Bool("pre", cfg.Update.Prerelease, "include pre-releases")
	checkOnly := fs.Bool("check", false, "only say whether a newer release exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if update.AssetName() == "" {
		return fmt.Errorf("no release binaries for %s/%s; build from source", runtime.GOOS, runtime.GOARCH)
	}
	rels, err := update.NewChecker(cfg.CacheDir, version).Releases(ctx, true)
	if err != nil {
		return err
	}
	r := update.Pick(rels, *pre)
	if r == nil {
		if latest := update.Pick(rels, true); !*pre && latest != nil {
			fmt.Printf("No stable release yet; the newest pre-release is %s.\n", latest.Tag)
			fmt.Println("Run `spotui update --pre`, or set prerelease = true under [update] in config.toml.")
			return nil
		}
		fmt.Println("No release with a binary for this platform yet.")
		return nil
	}
	if !update.Newer(r.Tag, version) {
		fmt.Printf("spotui %s is up to date.\n", version)
		return nil
	}
	kind := ""
	if r.Prerelease {
		kind = " (pre-release)"
	}
	if *checkOnly {
		fmt.Printf("spotui %s%s is available; you have %s.\n%s\n", r.Tag, kind, version, r.URL)
		return nil
	}

	exe, err := update.Executable()
	if err != nil {
		return err
	}
	if manager, how, ok := update.Packaged(exe); ok {
		fmt.Printf("spotui %s%s is out. This spotui was installed with %s, so update it there: %s\n", r.Tag, kind, manager, how)
		return nil
	}
	fmt.Printf("Installing spotui %s%s over %s (%s)…\n", r.Tag, kind, version, exe)
	if err := update.Install(ctx, nil, r, exe, version); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w\nThe folder of %s is not writable for you; run it with sudo or move spotui somewhere you own, such as ~/.local/bin", err, exe)
		}
		return err
	}
	fmt.Printf("Updated to %s. Restart spotui to use it.\n", r.Tag)
	return nil
}

func defaultShaderPath(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir, shaders.FileName)
}

// runShader writes the Windows Terminal shader and says where to point
// the profile at it.
func runShader(cfg *config.Config, path string) error {
	if path == "" {
		path = defaultShaderPath(cfg)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := shaders.Install(path); err != nil {
		return err
	}
	fmt.Println("Wrote", path)
	winPath := path
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		// Windows Terminal runs on the Windows side.
		if out, err := exec.Command("wslpath", "-w", path).Output(); err == nil {
			winPath = strings.TrimSpace(string(out))
		}
	}
	js, _ := json.Marshal(winPath)
	fmt.Printf("\nIn your Windows Terminal profile for spotui (Settings → Open JSON file):\n\n")
	// No padding and no scrollbar: the shader reaches neither, and they
	// would show as a plain frame and strip.
	fmt.Printf("    \"experimental.pixelShaderPath\": %s,\n    \"padding\": \"0\",\n    \"scrollbarState\": \"hidden\"\n\n", js)
	fmt.Println("Then turn on \"Windows Terminal shader\" in spotui's settings (,).")
	fmt.Println("spotui keeps this file up to date when it starts.")
	return nil
}
