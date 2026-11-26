package ui

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Playlist management: create, rename, describe and delete playlists, take
// tracks out (with undo) and move them. Everything goes through the
// embedded player to Spotify's playlist service, as adding tracks does.

// askState is a question in the footer: a line of text, or yes / no.
type askState struct {
	active  bool
	label   string
	hint    string
	confirm bool // yes / no, no text
	input   textinput.Model
	done    func(m *Model, value string) tea.Cmd
}

// ask asks for a line of text, prefilled with value.
func (m *Model) ask(label, hint, value string, done func(m *Model, value string) tea.Cmd) tea.Cmd {
	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 300
	in.SetValue(value)
	in.CursorEnd()
	m.asking = askState{active: true, label: label, hint: hint, input: in, done: done}
	m.overlay = overlayNone
	return m.asking.input.Focus()
}

// confirm asks a yes / no question; done runs on yes.
func (m *Model) confirm(label string, done func(m *Model) tea.Cmd) {
	m.asking = askState{active: true, label: label, confirm: true,
		done: func(m *Model, _ string) tea.Cmd { return done(m) }}
	m.overlay = overlayNone
}

func (m Model) updateAsk(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := &m.asking
	key := msg.String()
	if a.confirm {
		done := a.done
		m.asking = askState{}
		if key == "y" || key == "Y" || key == "enter" {
			return m, done(&m, "")
		}
		m.setFlash("Cancelled", false)
		return m, nil
	}
	switch key {
	case "esc", "ctrl+c":
		m.asking = askState{}
		return m, nil
	case "enter":
		done, v := a.done, strings.TrimSpace(a.input.Value())
		m.asking = askState{}
		return m, done(&m, v)
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return m, cmd
}

func (m Model) viewAsk(w int) string {
	a := m.asking
	if a.confirm {
		return ansi.Truncate(m.theme.title.Render(a.label)+"  "+
			m.theme.key.Render("y")+m.theme.dim.Render(" yes · ")+
			m.theme.key.Render("any other key")+m.theme.dim.Render(" no"), w, "…")
	}
	hint := a.hint
	if hint == "" {
		hint = "enter ok · esc cancel"
	}
	return ansi.Truncate(m.theme.dim.Render(a.label+" · "+hint), w, "…") + "\n" + a.input.View()
}

// playlistChangedMsg reports a finished playlist change.
type playlistChangedMsg struct {
	flash string
	err   error
	what  string
	// reloadLib refreshes the library's playlists (names, counts).
	reloadLib bool
	// deleted is a playlist that is gone: leave its page.
	deleted string
	// created is a new playlist, and addTrack a track to put in it.
	created, addTrack, addTitle string
}

func (m Model) playlistCall(what, flash string, reloadLib bool, fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := fn(ctx)
		return playlistChangedMsg{what: what, flash: flash, err: err, reloadLib: reloadLib}
	}
}

func (m *Model) handlePlaylistChanged(msg playlistChangedMsg) tea.Cmd {
	if msg.err != nil {
		m.setFlash(fmt.Sprintf("%s failed: %v", msg.what, msg.err), true)
		if cp := m.ctxPage; cp != nil && m.page == pageContext && cp.kind == "Playlist" {
			// The list shown may not be what Spotify has: read it again.
			cp.loading = true
			return m.loadContext(cp.uri)
		}
		return nil
	}
	if msg.flash != "" {
		m.setFlash(msg.flash, false)
	}
	var cmds []tea.Cmd
	if msg.deleted != "" {
		for cp := m.ctxPage; m.page == pageContext && cp != nil && cp.uri == msg.deleted; cp = m.ctxPage {
			m.goBack()
		}
	}
	if msg.created != "" && msg.addTrack != "" {
		eng, pl, track, title := m.eng, msg.created, msg.addTrack, msg.addTitle
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := eng.AddToPlaylist(ctx, pl, []string{track}); err != nil {
				return cmdResultMsg{what: "Add to playlist", err: err}
			}
			return flashMsg{text: "Added “" + title + "” to the new playlist"}
		})
	}
	if msg.reloadLib {
		m.lib.loaded[libPlaylists] = false
		if !m.lib.loading[libPlaylists] {
			m.lib.loading[libPlaylists] = true
			cmds = append(cmds, m.loadLibrary(libPlaylists))
		}
	}
	return tea.Batch(cmds...)
}

// --- create, rename, describe, delete ---

// newPlaylist asks for a name and a description and makes the playlist;
// with a track, the track goes into it.
func (m *Model) newPlaylist(trackURI, trackTitle string) tea.Cmd {
	public := m.cfg.Playlists.PublicByDefault
	return m.ask("New playlist: name", "enter next · esc cancel", "", func(m *Model, name string) tea.Cmd {
		if name == "" {
			m.setFlash("A playlist needs a name", true)
			return nil
		}
		return m.ask("Description for “"+name+"” (optional)", "enter create · esc cancel", "", func(m *Model, desc string) tea.Cmd {
			eng := m.eng
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				uri, err := eng.CreatePlaylist(ctx, name, desc, public)
				return playlistChangedMsg{what: "Create playlist", err: err, flash: "Created “" + name + "”",
					reloadLib: true, created: uri, addTrack: trackURI, addTitle: trackTitle}
			}
		})
	})
}

// playlistTargetItem is the playlist the edit keys act on: the one whose
// page is open, or the one selected in the library.
func (m Model) playlistTargetItem() *listItem {
	if cp := m.ctxPage; m.page == pageContext && cp != nil {
		if cp.kind != "Playlist" {
			return nil
		}
		return &listItem{kind: kindPlaylist, title: cp.title, uri: cp.uri, owned: cp.owned, editable: cp.editable, desc: cp.desc}
	}
	if m.page == pageLibrary && m.lib.tab == libPlaylists {
		if it := m.lib.lists[libPlaylists].selected(); it != nil && it.kind == kindPlaylist && strings.HasPrefix(it.uri, "spotify:playlist:") {
			return it
		}
	}
	return nil
}

func (m *Model) renamePlaylist() tea.Cmd {
	it := m.playlistTargetItem()
	if it == nil || !it.owned {
		m.setFlash("Pick one of your playlists to rename", true)
		return nil
	}
	uri, old := it.uri, it.title
	return m.ask("Rename “"+old+"”", "enter rename · esc cancel", old, func(m *Model, name string) tea.Cmd {
		if name == "" || name == old {
			return nil
		}
		if cp := m.ctxPage; cp != nil && cp.uri == uri {
			cp.title = name
		}
		eng := m.eng
		return m.playlistCall("Rename", "Renamed to “"+name+"”", true, func(ctx context.Context) error {
			return eng.EditPlaylist(ctx, uri, &name, nil)
		})
	})
}

func (m *Model) describePlaylist() tea.Cmd {
	it := m.playlistTargetItem()
	if it == nil || !it.owned {
		m.setFlash("Pick one of your playlists to describe", true)
		return nil
	}
	uri, title := it.uri, it.title
	return m.ask("Description of “"+title+"” (empty removes it)", "enter save · esc cancel", it.desc, func(m *Model, desc string) tea.Cmd {
		if cp := m.ctxPage; cp != nil && cp.uri == uri {
			cp.desc = desc
		}
		eng := m.eng
		return m.playlistCall("Description", "Description saved", true, func(ctx context.Context) error {
			return eng.EditPlaylist(ctx, uri, nil, &desc)
		})
	})
}

func (m *Model) deletePlaylist() tea.Cmd {
	it := m.playlistTargetItem()
	if it == nil {
		m.setFlash("Pick a playlist to delete", true)
		return nil
	}
	uri, title := it.uri, it.title
	question, done := "Delete “"+title+"”?", "Deleted “"+title+"”"
	if !it.owned {
		question, done = "Remove “"+title+"” from your library?", "Removed “"+title+"” from your library"
	}
	m.confirm(question, func(m *Model) tea.Cmd {
		eng := m.eng
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := eng.DeletePlaylist(ctx, uri)
			return playlistChangedMsg{what: "Delete playlist", err: err, flash: done, reloadLib: true, deleted: uri}
		}
	})
	return nil
}

// --- tracks in a playlist ---

// undoRemove is the last track taken out of a playlist, for U.
type undoRemove struct {
	playlist string
	item     listItem
	position int
	until    time.Time
}

// editablePage is the open playlist page when its tracks can be changed
// now; otherwise it says why not.
func (m *Model) editablePage() *contextPage {
	cp := m.ctxPage
	if m.page != pageContext || cp == nil || cp.kind != "Playlist" {
		return nil
	}
	switch {
	case !cp.editable:
		m.setFlash("This playlist is not yours to change", true)
	case cp.loading:
		m.setFlash("Wait until the playlist has loaded", true)
	case cp.list.filter != "":
		m.setFlash("Clear the filter (esc) to change the playlist", true)
	default:
		return cp
	}
	return nil
}

func (m *Model) removeFromPlaylist() tea.Cmd {
	cp := m.editablePage()
	if cp == nil {
		return nil
	}
	pos := cp.list.selectedIndex()
	if pos < 0 {
		return nil
	}
	it := cp.list.items[pos]
	items := append(append([]listItem{}, cp.list.items[:pos]...), cp.list.items[pos+1:]...)
	cp.list.setItems(items)
	cp.list.cursor = min(pos, max(len(cp.list.visible)-1, 0))
	cp.length--
	secs := max(m.cfg.Playlists.UndoSeconds, 0)
	m.undo = &undoRemove{playlist: cp.uri, item: it, position: pos, until: time.Now().Add(time.Duration(secs) * time.Second)}
	flash := "Removed “" + it.title + "”"
	if secs > 0 {
		flash += " · U brings it back"
	}
	eng, pl, uri := m.eng, cp.uri, it.uri
	m.lib.loaded[libPlaylists] = false // its track count, read again on the next visit
	return m.playlistCall("Remove from playlist", flash, false, func(ctx context.Context) error {
		return eng.RemoveFromPlaylist(ctx, pl, uri, pos)
	})
}

func (m *Model) undoRemoveFromPlaylist() tea.Cmd {
	u := m.undo
	if u == nil || time.Now().After(u.until) {
		m.undo = nil
		return nil
	}
	m.undo = nil
	if cp := m.ctxPage; cp != nil && cp.uri == u.playlist && cp.list.filter == "" {
		pos := min(u.position, len(cp.list.items))
		items := append(append(append([]listItem{}, cp.list.items[:pos]...), u.item), cp.list.items[pos:]...)
		cp.list.setItems(items)
		cp.list.cursor = pos
		cp.length++
	}
	eng := m.eng
	m.lib.loaded[libPlaylists] = false
	return m.playlistCall("Undo", "Put “"+u.item.title+"” back", false, func(ctx context.Context) error {
		return eng.InsertIntoPlaylist(ctx, u.playlist, []string{u.item.uri}, u.position)
	})
}

// moveInPlaylist moves the selected track up (-1) or down (+1).
func (m *Model) moveInPlaylist(dir int) tea.Cmd {
	cp := m.editablePage()
	if cp == nil {
		return nil
	}
	from := cp.list.selectedIndex()
	to := from + dir
	if from < 0 || to < 0 || to >= len(cp.list.items) {
		return nil
	}
	items := append([]listItem{}, cp.list.items...)
	items[from], items[to] = items[to], items[from]
	cp.list.setItems(items) // keeps the cursor on the moved track
	eng, pl, uri := m.eng, cp.uri, items[to].uri
	return m.playlistCall("Move", "", false, func(ctx context.Context) error {
		return eng.MovePlaylistItem(ctx, pl, uri, from, to)
	})
}

// handlePlaylistKey takes the playlist keys on the library's playlists and
// on a playlist's page.
func (m Model) handlePlaylistKey(key string) (bool, tea.Model, tea.Cmd) {
	onPlaylists := m.page == pageLibrary && m.lib.tab == libPlaylists
	onPage := m.page == pageContext && m.ctxPage != nil && m.ctxPage.kind == "Playlist"
	if !onPlaylists && !onPage {
		return false, m, nil
	}
	switch key {
	case "N":
		return true, m, m.newPlaylist("", "")
	case "e":
		return true, m, m.renamePlaylist()
	case "E":
		return true, m, m.describePlaylist()
	case "X":
		return true, m, m.deletePlaylist()
	}
	if !onPage {
		return false, m, nil
	}
	switch key {
	case "x", "delete":
		return true, m, m.removeFromPlaylist()
	case "U":
		return true, m, m.undoRemoveFromPlaylist()
	case "K", "shift+up":
		return true, m, m.moveInPlaylist(-1)
	case "J", "shift+down":
		return true, m, m.moveInPlaylist(+1)
	}
	return false, m, nil
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// plainDescription is a playlist description as text: Spotify keeps links
// in it as HTML, with entities.
func plainDescription(d string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTag.ReplaceAllString(d, ""))), " ")
}
