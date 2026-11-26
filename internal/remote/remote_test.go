package remote

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func sample() State {
	return State{
		Title: "Paper Lanterns", Artists: []string{"The Invented", "Someone Else"}, Album: "Night Shift",
		URI: "spotify:track:abc", DurationMs: 200_000, PositionMs: 60_000, PositionAt: t0,
		Status: "playing", Liked: true, Volume: 40, UpdatedAt: t0,
	}
}

func TestPositionMovesOnOnlyWhilePlaying(t *testing.T) {
	s := sample()
	if got := s.Position(t0.Add(5 * time.Second)); got != 65_000 {
		t.Fatalf("playing: got %d", got)
	}
	if got := s.Position(t0.Add(time.Hour)); got != s.DurationMs {
		t.Fatalf("past the end: got %d", got)
	}
	s.Status = "paused"
	if got := s.Position(t0.Add(5 * time.Second)); got != 60_000 {
		t.Fatalf("paused: got %d", got)
	}
}

func TestFormat(t *testing.T) {
	got := Format(sample(), "{icon} {artist} – {title} [{artists}] {pos}/{dur} {bar} {percent}% {liked} {state}", t0)
	want := "▶ The Invented – Paper Lanterns [The Invented, Someone Else] 1:00/3:20 ━━━─────── 30% ♥ playing"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := clock(3_725_000); got != "1:02:05" {
		t.Fatalf("clock: %q", got)
	}
}

func TestWaybar(t *testing.T) {
	b, err := Waybar(sample(), "{title}", t0)
	if err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if w["text"] != "Paper Lanterns" || w["class"] != "playing" || w["percentage"] != 30.0 ||
		!strings.Contains(w["tooltip"].(string), "Night Shift") {
		t.Fatalf("waybar: %s", b)
	}
}

func TestReadStatePicksTheFreshestAndSkipsStale(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, s State) string {
		p := filepath.Join(dir, name)
		b, _ := json.Marshal(s)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	old, fresh := sample(), sample()
	old.Title, old.UpdatedAt = "old", t0.Add(-5*time.Second)
	fresh.Title = "fresh"
	a, b := write("a.json", old), write("b.json", fresh)

	s, err := ReadState([]string{a, "", filepath.Join(dir, "missing"), b}, t0)
	if err != nil || s.Title != "fresh" {
		t.Fatalf("got %q, %v", s.Title, err)
	}
	if _, err := ReadState([]string{a, b}, t0.Add(time.Hour)); !errors.Is(err, ErrNoState) {
		t.Fatalf("stale files: %v", err)
	}
}

func TestPublisherWritesChangesAndRemovesOnClose(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "now.json"), filepath.Join(dir, "sub", "extra.json")}
	p := NewPublisher(append(paths, ""), t.Logf)
	s := sample()
	p.Publish(s)
	for _, path := range paths {
		waitFor(t, func() bool {
			got, err := ReadState([]string{path}, time.Now())
			return err == nil && got.Title == s.Title
		})
	}
	s.Title = "Second"
	p.Publish(s)
	p.Publish(s) // unchanged: dropped
	waitFor(t, func() bool {
		got, err := ReadState(paths[:1], time.Now())
		return err == nil && got.Title == "Second"
	})
	p.Close()
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s left behind: %v", path, err)
		}
	}
	var nilP *Publisher
	nilP.Publish(s) // no-op
	nilP.Close()
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("timed out")
}

func shortSocket(t *testing.T) string {
	// t.TempDir can be too long for a socket path on macOS.
	dir, err := os.MkdirTemp("", "spotui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, SocketName)
}

func TestControlSocket(t *testing.T) {
	path := shortSocket(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Send(ctx, path, Request{Cmd: "next"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("nothing listening: %v", err)
	}

	stop, err := Listen(path, func(_ context.Context, r Request) (string, error) {
		if r.Cmd == "fail" {
			return "", errors.New("nope")
		}
		return r.Cmd + ":" + strings.Join(r.Args, ","), nil
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Send(ctx, path, Request{Cmd: "volume", Args: []string{"-5"}}); err != nil || got != "volume:-5" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Send(ctx, path, Request{Cmd: "fail"}); err == nil || err.Error() != "nope" {
		t.Fatalf("error not passed on: %v", err)
	}
	if _, err := Listen(path, nil, t.Logf); !errors.Is(err, ErrRunning) {
		t.Fatalf("second listener: %v", err)
	}
	stop()
	if _, err := Send(ctx, path, Request{Cmd: "next"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("after stop: %v", err)
	}
}

func TestListenTakesOverAStaleSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a plain file is no stale socket on Windows")
	}
	path := shortSocket(t)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stop, err := Listen(path, func(context.Context, Request) (string, error) { return "ok", nil }, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if got, err := Send(context.Background(), path, Request{Cmd: "x"}); err != nil || got != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestHookEnvAndRun(t *testing.T) {
	tr := Track{Title: "Paper Lanterns", Artists: []string{"A", "B"}, DurationMs: 1234, Accent: "#112233"}
	env := strings.Join(tr.Env("track_change"), "\n")
	for _, want := range []string{"SPOTUI_TITLE=Paper Lanterns", "SPOTUI_ARTIST=A, B", "SPOTUI_DURATION_MS=1234", "SPOTUI_ACCENT=#112233", "SPOTUI_EVENT=track_change"} {
		if !strings.Contains(env, want) {
			t.Errorf("missing %s", want)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	out := filepath.Join(t.TempDir(), "out")
	RunHook(`printf '%s|%s' "$SPOTUI_ARTIST" "$SPOTUI_TITLE" > "`+out+`"`, "track_change", tr, t.Logf)
	waitFor(t, func() bool {
		b, _ := os.ReadFile(out)
		return string(b) == "A, B|Paper Lanterns"
	})
	RunHook("   ", "track_change", tr, t.Fatalf) // empty: nothing runs
}
