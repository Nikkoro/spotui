package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/remote"
)

// RemoteMsg is a command from another terminal (`spotui next`, ...), handed
// in through the control socket. Reply is buffered and gets one answer.
type RemoteMsg struct {
	Cmd   string
	Args  []string
	Reply chan<- RemoteReply
}

// RemoteReply answers a RemoteMsg: what to print, or what went wrong.
type RemoteReply struct {
	Text string
	Err  error
}

// RemoteCommands are the commands the socket takes, for usage texts.
var RemoteCommands = []string{"play", "pause", "toggle", "next", "prev", "like", "unlike", "volume", "radio"}

// WithRemote keeps the state file of `spotui now` through p.
func (m Model) WithRemote(p *remote.Publisher) Model {
	m.remote = p
	return m
}

func (m Model) handleRemote(msg RemoteMsg) (tea.Model, tea.Cmd) {
	reply := func(text string, err error) {
		select {
		case msg.Reply <- RemoteReply{Text: text, Err: err}:
		default:
		}
	}
	if m.phase != phaseReady {
		reply("", errors.New("spotui is not logged in yet"))
		return m, nil
	}
	e := m.eng
	// run carries out the command off the UI goroutine and answers with
	// the player's own result.
	run := func(what string, fn func(ctx context.Context) error) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := fn(ctx)
			reply("", err)
			return cmdResultMsg{what: what, err: err}
		}
	}
	arg := strings.TrimSpace(strings.Join(msg.Args, " "))

	switch msg.Cmd {
	case "toggle":
		m.setPosition(m.position())
		m.paused = !m.paused
		return m, run("Play/pause", e.PlayPause)
	case "pause":
		return m, run("Pause", e.Pause)
	case "play":
		if arg == "" {
			return m, run("Play", e.Resume)
		}
		uri, err := parseSpotifyURI(arg)
		if err != nil {
			reply("", err)
			return m, nil
		}
		m.setFlash("Loading "+uri, false)
		return m, run("Play", func(ctx context.Context) error { return e.Play(ctx, uri) })
	case "next":
		return m, run("Next", e.Next)
	case "prev", "previous":
		return m, run("Previous", e.Prev)
	case "like", "unlike":
		uri := m.currentURI()
		if uri == "" {
			reply("", errors.New("nothing is playing"))
			return m, nil
		}
		on := msg.Cmd == "like"
		m.liked[uri] = on
		if on {
			m.likedNowURI, m.likedNowAt = uri, time.Now()
			m.setFlash("Added to Liked Songs", false)
		} else {
			m.setFlash("Removed from Liked Songs", false)
		}
		return m, run("Like", func(ctx context.Context) error { return e.SetLiked(ctx, []string{uri}, on) })
	case "radio":
		seed, title := m.currentURI(), ""
		if m.track != nil {
			title = m.track.Name
		}
		if arg != "" {
			uri, err := parseSpotifyURI(arg)
			if err != nil {
				reply("", err)
				return m, nil
			}
			seed, title = uri, uri
		}
		if !radioable(seed) {
			reply("", errors.New("radio needs a song, artist, album or playlist"))
			return m, nil
		}
		eng := m.eng
		var recent []string
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			station, err := eng.Radio(ctx, seed, recent)
			if err != nil {
				reply("", err)
			} else {
				reply("", nil)
			}
			return radioMsg{seed: seed, title: title, station: station, err: err}
		}
	case "volume":
		vol, relative, err := parseVolume(arg)
		if err != nil {
			reply("", err)
			return m, nil
		}
		steps := int32(vol * int(max(m.volumeMax, 1)) / 100)
		return m, run("Volume", func(ctx context.Context) error { return e.SetVolume(ctx, steps, relative) })
	}
	reply("", fmt.Errorf("unknown command %q (known: %s)", msg.Cmd, strings.Join(RemoteCommands, ", ")))
	return m, nil
}

// parseVolume reads "40" (set to 40 %), "+5" or "-5" (relative).
func parseVolume(s string) (n int, relative bool, err error) {
	relative = strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
	n, err = strconv.Atoi(strings.TrimSuffix(s, "%"))
	if err != nil || (!relative && (n < 0 || n > 100)) {
		return 0, false, fmt.Errorf("volume takes 0-100, +N or -N, not %q", s)
	}
	return n, relative, nil
}

// remoteState is what the state file shows of the player now.
func (m Model) remoteState() remote.State {
	s := remote.State{
		Status:     "stopped",
		PositionMs: m.posMs,
		PositionAt: m.posAt,
		Shuffle:    m.shuffle,
		Repeat:     "off",
		Accent:     m.theme.accentHex,
		Device:     m.cfg.DeviceName,
	}
	if m.volumeMax > 0 {
		s.Volume = int(m.volume * 100 / m.volumeMax)
	}
	switch {
	case m.repeatTrack:
		s.Repeat = "track"
	case m.repeatCtx:
		s.Repeat = "context"
	}
	t := m.track
	if t == nil || m.phase != phaseReady {
		return s
	}
	switch {
	case m.stopped:
	case m.paused:
		s.Status = "paused"
	case m.buffering:
		// The position stands still while loading, as in m.position.
		s.Status = "paused"
	default:
		s.Status = "playing"
	}
	s.Title, s.Album, s.URI = t.Name, t.AlbumName, t.Uri
	s.Artists = t.ArtistNames
	s.URL = spotifyURL(t.Uri)
	s.DurationMs = int64(t.Duration)
	s.Liked = m.liked[t.Uri]
	if t.AlbumCoverUrl != nil {
		s.CoverURL = *t.AlbumCoverUrl
	}
	return s
}

// publishState brings the state file up to date; the publisher drops
// states that did not change.
func (m Model) publishState() {
	if m.remote != nil {
		m.remote.Publish(m.remoteState())
	}
}

// runTrackHook runs [hooks] on_track_change for the track now playing.
func (m Model) runTrackHook() {
	cmd := m.cfg.Hooks.OnTrackChange
	t := m.track
	if strings.TrimSpace(cmd) == "" || t == nil {
		return
	}
	ht := remote.Track{
		Title:      t.Name,
		Album:      t.AlbumName,
		URI:        t.Uri,
		URL:        spotifyURL(t.Uri),
		Artists:    t.ArtistNames,
		DurationMs: int64(t.Duration),
		Accent:     m.theme.accentHex,
	}
	if t.AlbumCoverUrl != nil {
		ht.CoverURL = *t.AlbumCoverUrl
		ht.CoverPath = m.covers.File(ht.CoverURL)
	}
	remote.RunHook(cmd, "track_change", ht, m.eng.Debugf)
}
