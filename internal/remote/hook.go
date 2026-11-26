package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// HookTimeout is how long a hook may run before it is stopped.
const HookTimeout = 30 * time.Second

// Track is what a hook is told about the new track.
type Track struct {
	Title, Album, URI, URL string
	Artists                []string
	DurationMs             int64
	CoverURL, CoverPath    string // CoverPath: the cached cover (JPEG), if on disk
	Accent                 string // #rrggbb
}

// Env lists the variables a hook sees, as KEY=value.
func (t Track) Env(event string) []string {
	return []string{
		"SPOTUI_EVENT=" + event,
		"SPOTUI_TITLE=" + t.Title,
		"SPOTUI_ARTIST=" + strings.Join(t.Artists, ", "),
		"SPOTUI_ALBUM=" + t.Album,
		"SPOTUI_URI=" + t.URI,
		"SPOTUI_URL=" + t.URL,
		fmt.Sprintf("SPOTUI_DURATION_MS=%d", t.DurationMs),
		"SPOTUI_COVER_URL=" + t.CoverURL,
		"SPOTUI_COVER_PATH=" + t.CoverPath,
		"SPOTUI_ACCENT=" + t.Accent,
	}
}

// RunHook runs command through the shell (sh, or cmd on Windows) in the
// background with the track in its environment. It returns at once;
// failures and output go to logf.
func RunHook(command, event string, t Track, logf func(string, ...any)) {
	if strings.TrimSpace(command) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), HookTimeout)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/C", command)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", command)
		}
		cmd.Env = append(os.Environ(), t.Env(event)...)
		// A child left in the background must not keep the hook waiting
		// on its output.
		cmd.WaitDelay = 2 * time.Second
		out, err := cmd.CombinedOutput()
		if logf == nil {
			return
		}
		msg := strings.TrimSpace(string(out))
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		switch {
		case ctx.Err() != nil:
			logf("hook %s: stopped after %s", event, HookTimeout)
		case err != nil:
			logf("hook %s failed: %v %s", event, err, msg)
		case msg != "":
			logf("hook %s: %s", event, msg)
		}
	}()
}
