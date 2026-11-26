package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
)

func TestParseVolume(t *testing.T) {
	for in, want := range map[string]struct {
		n   int
		rel bool
	}{"40": {40, false}, "+5": {5, true}, "-10": {-10, true}, "70%": {70, false}} {
		n, rel, err := parseVolume(in)
		if err != nil || n != want.n || rel != want.rel {
			t.Errorf("%q: got %d %v %v", in, n, rel, err)
		}
	}
	for _, bad := range []string{"", "loud", "120", "-"} {
		if _, _, err := parseVolume(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func remoteModel(t *testing.T) Model {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, cfg, nil)
	m.phase = phaseReady
	return m
}

func TestRemoteState(t *testing.T) {
	m := remoteModel(t)
	if s := m.remoteState(); s.Status != "stopped" || s.Title != "" {
		t.Fatalf("no track: %+v", s)
	}
	m.track = &daemon.ApiTrack{Uri: "spotify:track:abc", Name: "Paper Lanterns", ArtistNames: []string{"The Invented"}, Duration: 200000}
	m.stopped, m.paused = false, false
	m.volume, m.volumeMax = 50, 100
	m.repeatCtx = true
	m.liked["spotify:track:abc"] = true
	m.setPosition(1000)
	s := m.remoteState()
	if s.Status != "playing" || s.Title != "Paper Lanterns" || !s.Liked || s.Volume != 50 || s.Repeat != "context" ||
		s.URL != "https://open.spotify.com/track/abc" || s.PositionMs != 1000 {
		t.Fatalf("got %+v", s)
	}
	// The state stays the same while the position only moves on, so the
	// file is not rewritten every frame.
	time.Sleep(5 * time.Millisecond)
	if s2 := m.remoteState(); s2.PositionMs != s.PositionMs || !s2.PositionAt.Equal(s.PositionAt) {
		t.Fatal("state changes with time alone")
	}
	m.paused = true
	if s := m.remoteState(); s.Status != "paused" {
		t.Fatalf("paused: %s", s.Status)
	}
}

func TestRemoteRejectsWhatItCannotDo(t *testing.T) {
	m := remoteModel(t)
	ask := func(m Model, cmd string, args ...string) error {
		reply := make(chan RemoteReply, 1)
		_, c := m.handleRemote(RemoteMsg{Cmd: cmd, Args: args, Reply: reply})
		if c != nil {
			t.Fatalf("%s: wants to call the player", cmd)
		}
		return (<-reply).Err
	}
	if err := ask(m, "dance"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown: %v", err)
	}
	if err := ask(m, "like"); err == nil {
		t.Fatal("like with nothing playing")
	}
	if err := ask(m, "volume", "loud"); err == nil {
		t.Fatal("bad volume")
	}
	if err := ask(m, "play", "not a link"); err == nil {
		t.Fatal("bad link")
	}
	m.phase = phaseAuth
	if err := ask(m, "next"); err == nil {
		t.Fatal("next before login")
	}
}
