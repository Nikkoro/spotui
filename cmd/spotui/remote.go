package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/remote"
	"github.com/Nikkoro/spotui/internal/ui"
)

func socketPath(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir, remote.SocketName)
}

// statePaths are where the running spotui keeps the state of `spotui now`:
// the cache folder, and status_file when set.
func statePaths(cfg *config.Config) []string {
	paths := []string{filepath.Join(cfg.CacheDir, remote.StateFileName)}
	if p := expandHome(strings.TrimSpace(cfg.Control.StatusFile)); p != "" && p != paths[0] {
		paths = append(paths, p)
	}
	return paths
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~"); ok && (rest == "" || rest[0] == '/' || rest[0] == '\\') {
		if home, err := os.UserHomeDir(); err == nil {
			return home + rest
		}
	}
	return p
}

// remoteHandler hands socket commands to the interface running now.
func remoteHandler(prog *atomic.Pointer[tea.Program]) remote.Handler {
	return func(ctx context.Context, req remote.Request) (string, error) {
		p := prog.Load()
		if p == nil {
			return "", errors.New("spotui is restarting the player; try again in a moment")
		}
		reply := make(chan ui.RemoteReply, 1)
		go p.Send(ui.RemoteMsg{Cmd: req.Cmd, Args: req.Args, Reply: reply})
		select {
		case r := <-reply:
			return r.Text, r.Err
		case <-ctx.Done():
			return "", errors.New("spotui did not answer in time")
		}
	}
}

// runRemote sends one command to the running spotui.
func runRemote(cfg *config.Config, cmd string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	text, err := remote.Send(ctx, socketPath(cfg), remote.Request{Cmd: cmd, Args: args})
	if err != nil {
		return err
	}
	if text != "" {
		fmt.Println(text)
	}
	return nil
}

// jsonFlag is --json, or --json=waybar for waybar's custom modules.
type jsonFlag string

func (j *jsonFlag) String() string   { return string(*j) }
func (j *jsonFlag) IsBoolFlag() bool { return true }
func (j *jsonFlag) Set(v string) error {
	switch v {
	case "true", "waybar":
		*j = jsonFlag(v)
	case "false":
		*j = ""
	default:
		return fmt.Errorf("--json takes no value or waybar, not %q", v)
	}
	return nil
}

// runNow prints what the running spotui plays, from its state file alone:
// no Spotify calls, so status bars can run it every second. It prints
// nothing when spotui is not running.
func runNow(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("now", flag.ContinueOnError)
	format := fs.String("format", remote.DefaultFormat, "placeholders: {title} {artist} {artists} {album} {pos} {dur} {remaining} {bar} {percent} {state} {icon} {liked} {volume} {accent} {uri} {url}")
	file := fs.String("file", "", "read this state file instead of the usual places")
	var asJSON jsonFlag
	fs.Var(&asJSON, "json", "print JSON; --json=waybar for waybar (text, tooltip, class)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := statePaths(cfg)
	if *file != "" {
		paths = []string{expandHome(*file)}
	}
	now := time.Now()
	s, err := remote.ReadState(paths, now)
	if errors.Is(err, remote.ErrNoState) || (err == nil && s.Title == "") {
		if asJSON == "waybar" {
			fmt.Println(`{"text":"","class":"stopped"}`)
		}
		return nil
	}
	if err != nil {
		return err
	}
	switch asJSON {
	case "waybar":
		b, err := remote.Waybar(s, *format, now)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	case "true":
		s.PositionMs, s.PositionAt = s.Position(now), now
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	default:
		fmt.Println(remote.Format(s, *format, now))
	}
	return nil
}
