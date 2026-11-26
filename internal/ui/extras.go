package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- go to the album / artist of the playing track ---

func (m *Model) goToAlbum() tea.Cmd {
	t := m.track
	if t == nil || t.AlbumUri == "" {
		return nil
	}
	kind := kindAlbum
	if isShow(t.AlbumUri) {
		kind = kindShow // an episode's show
	}
	return m.openContext(listItem{kind: kind, title: t.AlbumName, uri: t.AlbumUri})
}

func (m *Model) goToArtist() tea.Cmd {
	t := m.track
	if t == nil || len(t.ArtistUris) == 0 {
		return nil
	}
	name := ""
	if len(t.ArtistNames) > 0 {
		name = t.ArtistNames[0]
	}
	return m.openContext(listItem{kind: kindArtist, title: name, uri: t.ArtistUris[0]})
}

// --- add to playlist ---

// playlistTarget is the track "add to playlist" acts on: the selected track
// in a list, otherwise the one playing.
func (m Model) playlistTarget() (uri, title string) {
	if l := m.activeList(); l != nil && m.page != pageNowPlaying {
		if it := l.selected(); it != nil && it.kind == kindTrack && strings.HasPrefix(it.uri, "spotify:track:") {
			return it.uri, it.title
		}
	}
	if m.track != nil {
		return m.track.Uri, m.track.Name
	}
	return "", ""
}

// newPlaylistURI marks the picker's "+ New playlist" entry.
const newPlaylistURI = "spotui:new-playlist"

func (m *Model) openPlaylistPicker() tea.Cmd {
	uri, title := m.playlistTarget()
	if uri == "" {
		m.setFlash("Nothing to add", true)
		return nil
	}
	m.picker.trackURI, m.picker.trackTitle = uri, title

	items := []listItem{{kind: kindPlaylist, title: "+ New playlist…", subtitle: "make one and add the track", uri: newPlaylistURI}}
	for _, it := range m.lib.lists[libPlaylists].items {
		if it.kind == kindPlaylist && it.editable {
			items = append(items, it)
		}
	}
	m.picker.list.setItems(items)
	m.picker.list.setFilter("")
	m.picker.filter.SetValue("")
	m.overlay = overlayPlaylists

	// The playlists come from the library; load it if it is not there yet.
	if !m.lib.loaded[libPlaylists] && !m.lib.loading[libPlaylists] {
		m.lib.loading[libPlaylists] = true
		return tea.Batch(m.loadLibrary(libPlaylists), m.picker.filter.Focus())
	}
	return m.picker.filter.Focus()
}

func (m Model) updatePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "ctrl+p":
		m.picker.list.move(-1)
		return m, nil
	case "down", "ctrl+n", "tab":
		m.picker.list.move(1)
		return m, nil
	case "enter":
		it := m.picker.list.selected()
		if it == nil {
			return m, nil
		}
		m.overlay = overlayNone
		if it.uri == newPlaylistURI {
			return m, m.newPlaylist(m.picker.trackURI, m.picker.trackTitle)
		}
		eng, pl, track := m.eng, it.uri, m.picker.trackURI
		name, title := it.title, m.picker.trackTitle
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := eng.AddToPlaylist(ctx, pl, []string{track}); err != nil {
				return cmdResultMsg{what: "Add to playlist", err: err}
			}
			return flashMsg{text: fmt.Sprintf("Added “%s” to %s", title, name)}
		}
	}
	var cmd tea.Cmd
	m.picker.filter, cmd = m.picker.filter.Update(msg)
	m.picker.list.setFilter(m.picker.filter.Value())
	return m, cmd
}

func (m Model) viewPlaylistPicker(w, h int) string {
	cw := clamp(w-16, 40, 70)
	lines := []string{
		m.theme.title.Render("Add to playlist"),
		m.theme.dim.Render(truncate("“"+m.picker.trackTitle+"”", cw)),
		"",
		m.picker.filter.View(),
		"",
	}
	switch {
	case len(m.picker.list.items) <= 1 && m.lib.loading[libPlaylists]:
		lines = append(lines, m.loadingLine("Loading your playlists…"))
	default:
		lines = append(lines, m.picker.list.view(m.theme, cw, clamp(h-14, 3, 15), ""))
	}
	lines = append(lines, "", m.theme.dim.Render("type to filter · enter adds · esc close"))
	return m.theme.card.Render(strings.Join(lines, "\n"))
}

// --- sleep timer ---

var sleepPresets = []time.Duration{15 * time.Minute, 30 * time.Minute, 45 * time.Minute, time.Hour, 90 * time.Minute}

type sleepDueMsg struct{ id int }

// cycleSleep steps through off → 15 → 30 → 45 → 60 → 90 min → end of track → off.
func (m *Model) cycleSleep() tea.Cmd {
	m.sleepStep = (m.sleepStep + 1) % (len(sleepPresets) + 2)
	m.sleepID++
	m.sleepAt, m.sleepEndOfTrack = time.Time{}, false

	switch {
	case m.sleepStep == 0:
		m.setFlash("Sleep timer off", false)
		return nil
	case m.sleepStep == len(sleepPresets)+1:
		m.sleepEndOfTrack = true
		m.setFlash("Sleep at the end of this track", false)
		return nil
	}
	d := sleepPresets[m.sleepStep-1]
	m.sleepAt = time.Now().Add(d)
	m.setFlash(fmt.Sprintf("Sleep timer: %d min", int(d.Minutes())), false)
	id := m.sleepID
	return tea.Tick(d, func(time.Time) tea.Msg { return sleepDueMsg{id: id} })
}

func (m *Model) sleepNow() tea.Cmd {
	m.sleepAt, m.sleepEndOfTrack, m.sleepStep = time.Time{}, false, 0
	m.setFlash("Sleep timer: paused playback. Good night", false)
	return call("Pause", m.eng.Pause)
}

func (m Model) sleepLabel() string {
	switch {
	case m.sleepEndOfTrack:
		return "☾ end of track"
	case !m.sleepAt.IsZero():
		left := time.Until(m.sleepAt).Round(time.Minute)
		return fmt.Sprintf("☾ %dm", int(left.Minutes()))
	}
	return ""
}

type flashMsg struct {
	text string
	err  bool
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w < 1 {
		return ""
	}
	return string(r[:w-1]) + "…"
}

// playWhole plays a playlist, album or artist from the start, optionally
// shuffled.
func (m *Model) playWhole(uri, title string, shuffle bool) tea.Cmd {
	eng := m.eng
	if shuffle {
		m.setFlash("Shuffling "+title, false)
	} else {
		m.setFlash("Playing "+title, false)
	}
	return call("Play", func(ctx context.Context) error {
		if err := eng.SetShuffle(ctx, shuffle); err != nil {
			return err
		}
		return eng.PlayContext(ctx, uri, "")
	})
}
