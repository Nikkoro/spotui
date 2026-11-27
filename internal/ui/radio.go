package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Radio: endless music like a track, an artist, an album or a playlist, as
// the Spotify apps' "Go to radio". The player finds the seed's station and
// plays it as a context, so it carries on as autoplay does.

// radioCacheTTL is how long a seed's station is remembered.
const radioCacheTTL = time.Hour

type radioEntry struct {
	station string
	at      time.Time
}

type radioMsg struct {
	seed, title, station string
	err                  error
}

// radioSeed is what S starts a radio of: the selected row of a list, or
// the playing track on Now Playing.
func (m Model) radioSeed() (uri, title string) {
	if l := m.activeList(); l != nil && m.page != pageNowPlaying {
		if it := l.selected(); it != nil && radioable(it.uri) {
			return it.uri, it.title
		}
	}
	if m.track != nil && radioable(m.track.Uri) {
		return m.track.Uri, m.track.Name
	}
	return "", ""
}

// radioable: songs, artists, albums and playlists have radios; episodes,
// shows and the user's collections do not.
func radioable(uri string) bool {
	for _, p := range []string{"spotify:track:", "spotify:artist:", "spotify:album:", "spotify:playlist:"} {
		if strings.HasPrefix(uri, p) {
			return true
		}
	}
	return false
}

// startRadio plays the radio of seed.
func (m *Model) startRadio(seed, title string) tea.Cmd {
	if !radioable(seed) {
		m.setFlash("Radio is for songs, artists, albums and playlists", true)
		return nil
	}
	m.setFlash("Starting the radio of “"+title+"”…", false)
	if e, ok := m.radios[seed]; ok && time.Since(e.at) < radioCacheTTL {
		return m.playRadio(seed, title, e.station)
	}

	// A few of the seed's tracks, when its page is open: the autoplay
	// service makes a station of them if the seed has none of its own.
	var recent []string
	if cp := m.ctxPage; cp != nil && cp.uri == seed {
		for _, it := range cp.list.items {
			if strings.HasPrefix(it.uri, "spotify:track:") {
				recent = append(recent, it.uri)
				if len(recent) == 5 {
					break
				}
			}
		}
	}
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		station, err := eng.Radio(ctx, seed, recent)
		return radioMsg{seed: seed, title: title, station: station, err: err}
	}
}

func (m *Model) handleRadio(msg radioMsg) tea.Cmd {
	if msg.err != nil {
		m.setFlash("No radio for “"+msg.title+"”: "+msg.err.Error(), true)
		return nil
	}
	m.radios[msg.seed] = radioEntry{station: msg.station, at: time.Now()}
	return m.playRadio(msg.seed, msg.title, msg.station)
}

func (m *Model) playRadio(seed, title, station string) tea.Cmd {
	eng := m.eng
	m.setFlash("Radio: “"+title+"”", false)
	return call("Radio", func(ctx context.Context) error {
		// A song's radio starts with the song, as in the Spotify apps.
		first := ""
		if strings.HasPrefix(seed, "spotify:track:") {
			first = seed
		}
		return eng.PlayRadio(ctx, station, first)
	})
}
