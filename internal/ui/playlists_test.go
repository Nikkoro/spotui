package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func playlistPage(t *testing.T, editable bool) Model {
	m := remoteModel(t)
	m.width, m.height = 120, 40
	_ = m.openContext(listItem{kind: kindPlaylist, title: "Night drives", uri: "spotify:playlist:p1", editable: editable, owned: editable})
	cp := m.ctxPage
	cp.loading = false
	cp.list.setItems([]listItem{
		{kind: kindTrack, title: "One", uri: "spotify:track:1"},
		{kind: kindTrack, title: "Two", uri: "spotify:track:2"},
		{kind: kindTrack, title: "Three", uri: "spotify:track:3"},
	})
	cp.length = 3
	return m
}

func titles(l listModel) string {
	s := ""
	for _, it := range l.items {
		s += it.title + " "
	}
	return s
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.handleKey(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func TestRemoveFromPlaylistAndUndo(t *testing.T) {
	m := playlistPage(t, true)
	m = press(m, "j", "x")
	if got := titles(m.ctxPage.list); got != "One Three " {
		t.Fatalf("after x: %q", got)
	}
	if m.undo == nil || m.undo.position != 1 || m.undo.item.uri != "spotify:track:2" {
		t.Fatalf("undo: %+v", m.undo)
	}
	m = press(m, "U")
	if got := titles(m.ctxPage.list); got != "One Two Three " || m.undo != nil {
		t.Fatalf("after U: %q", got)
	}
}

func TestMoveInPlaylist(t *testing.T) {
	m := playlistPage(t, true)
	m = press(m, "J")
	if got := titles(m.ctxPage.list); got != "Two One Three " {
		t.Fatalf("after J: %q", got)
	}
	if it := m.ctxPage.list.selected(); it == nil || it.title != "One" {
		t.Fatal("cursor did not follow the track")
	}
	m = press(m, "K", "K")
	if got := titles(m.ctxPage.list); got != "One Two Three " {
		t.Fatalf("after K K: %q", got)
	}
}

func TestPlaylistEditsRefusedWhereTheyCannotWork(t *testing.T) {
	m := playlistPage(t, false)
	m = press(m, "x")
	if len(m.ctxPage.list.items) != 3 || !m.flashErr {
		t.Fatal("removed from a playlist that is not ours")
	}
	m = playlistPage(t, true)
	m.ctxPage.list.setFilter("t")
	m = press(m, "J")
	if got := titles(m.ctxPage.list); got != "One Two Three " {
		t.Fatalf("moved while filtered: %q", got)
	}
}

func TestNewPlaylistAsksNameThenDescription(t *testing.T) {
	m := playlistPage(t, true)
	m = press(m, "N")
	if !m.asking.active || m.asking.confirm {
		t.Fatal("no name prompt")
	}
	type_ := func(m Model, s string) Model {
		for _, r := range s {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = next.(Model)
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		_ = cmd
		return m
	}
	m = type_(m, "Road")
	if !m.asking.active || m.asking.label != "Description for “Road” (optional)" {
		t.Fatalf("no description prompt: %+v", m.asking.label)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.asking.active || cmd == nil {
		t.Fatal("description did not finish the prompt")
	}
}

func TestDeleteAsksFirst(t *testing.T) {
	m := playlistPage(t, true)
	m = press(m, "X")
	if !m.asking.active || !m.asking.confirm || m.asking.label != "Delete “Night drives”?" {
		t.Fatalf("no confirmation: %+v", m.asking)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(Model)
	if m.asking.active || cmd != nil {
		t.Fatal("n did not cancel")
	}
	m = playlistPage(t, false)
	m = press(m, "X")
	if m.asking.label != "Remove “Night drives” from your library?" {
		t.Fatalf("someone else's playlist: %q", m.asking.label)
	}
}

func TestPickerOffersANewPlaylist(t *testing.T) {
	m := playlistPage(t, true)
	_ = m.openPlaylistPicker()
	if it := m.picker.list.selected(); it == nil || it.uri != newPlaylistURI {
		t.Fatal("no new playlist entry first")
	}
}

func TestContextPollBacksOffWithoutProgress(t *testing.T) {
	cp := &contextPage{}
	if d := cp.nextPoll(0, 0, false); d != ctxPollFast {
		t.Fatalf("first: %v", d)
	}
	var d time.Duration
	for range 10 {
		d = cp.nextPoll(0, 0, false)
	}
	if d != ctxPollSlow {
		t.Fatalf("stuck listing still polled every %v", d)
	}
	if d := cp.nextPoll(40, 10, true); d != ctxPollFast {
		t.Fatalf("progress: %v", d)
	}
	for i := range ctxStallLimit {
		if cp.noteProgress(40, 39, true) != (i == ctxStallLimit) {
			t.Fatalf("stalled after %d", i)
		}
	}
	if !cp.noteProgress(40, 39, true) {
		t.Fatal("a stuck listing never gave up")
	}
}

func TestPlainDescription(t *testing.T) {
	got := plainDescription(`Songs for <a href="spotify:user:x">late</a> drives &amp; rain`)
	if got != "Songs for late drives & rain" {
		t.Fatalf("got %q", got)
	}
}
