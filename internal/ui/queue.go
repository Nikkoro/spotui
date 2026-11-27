package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/daemon"
)

// Queue editing: take entries out of the user's queue (with undo), move
// them, clear it, and put a track in to play next. The player changes its
// own queue and Spotify Connect shows the change on every device.

// queueUndo is the last entry taken out of the queue, for U.
type queueUndo struct {
	uri, title string
	index      int
	until      time.Time
}

// queuePos is the index of a list row among the user's queue entries, or
// -1 when it is not one.
func queuePos(l *listModel, idx int) int {
	if idx < 0 || idx >= len(l.items) || !l.items[idx].queued {
		return -1
	}
	n := 0
	for i := 0; i < idx; i++ {
		if l.items[i].queued {
			n++
		}
	}
	return n
}

func (m Model) editQueue(what, flash string, edit daemon.ApiQueueEdit) tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := eng.EditQueue(ctx, edit); err != nil {
			return cmdResultMsg{what: what, err: err}
		}
		if flash != "" {
			return flashMsg{text: flash}
		}
		return nil
	}
}

// queueEntry is the selected row on the queue page when it is in the
// user's queue; otherwise it says why nothing can change.
func (m *Model) queueEntry() (*listItem, int, int) {
	l := &m.queue.list
	if l.filter != "" {
		m.setFlash("Clear the filter (esc) to change the queue", true)
		return nil, -1, -1
	}
	idx := l.selectedIndex()
	pos := queuePos(l, idx)
	if pos < 0 {
		m.setFlash("Only songs in your queue can be taken out or moved", true)
		return nil, -1, -1
	}
	return &l.items[idx], idx, pos
}

func (m *Model) removeFromQueue() tea.Cmd {
	it, idx, pos := m.queueEntry()
	if it == nil {
		return nil
	}
	gone := *it
	l := &m.queue.list
	items := append(append([]listItem{}, l.items[:idx]...), l.items[idx+1:]...)
	l.setItems(items)
	l.cursor = min(l.cursor, max(len(l.visible)-1, 0))
	secs := max(m.cfg.Playlists.UndoSeconds, 0)
	m.queueUndo = &queueUndo{uri: gone.uri, title: gone.title, index: pos, until: time.Now().Add(time.Duration(secs) * time.Second)}
	flash := "Took “" + gone.title + "” out of the queue"
	if secs > 0 {
		flash += " · U brings it back"
	}
	m.setFlash(flash, false)
	return m.editQueue("Remove from queue", "", daemon.ApiQueueEdit{Op: daemon.QueueRemove, Uid: gone.uid, Uri: gone.uri})
}

func (m *Model) undoRemoveFromQueue() tea.Cmd {
	u := m.queueUndo
	m.queueUndo = nil
	if u == nil || time.Now().After(u.until) {
		return nil
	}
	return m.editQueue("Undo", "Put “"+u.title+"” back in the queue",
		daemon.ApiQueueEdit{Op: daemon.QueueInsert, Uri: u.uri, To: u.index})
}

// moveInQueue moves the selected queue entry up (-1) or down (+1) among the
// queue's entries.
func (m *Model) moveInQueue(dir int) tea.Cmd {
	it, idx, pos := m.queueEntry()
	if it == nil {
		return nil
	}
	l := &m.queue.list
	j := idx + dir
	if j < 0 || j >= len(l.items) || !l.items[j].queued {
		return nil // the end of the queue
	}
	moved := *it
	items := append([]listItem{}, l.items...)
	items[idx], items[j] = items[j], items[idx]
	l.setItems(items) // keeps the cursor on the moved entry
	return m.editQueue("Move", "", daemon.ApiQueueEdit{Op: daemon.QueueMove, Uid: moved.uid, Uri: moved.uri, To: pos + dir})
}

func (m *Model) clearQueue() tea.Cmd {
	n := 0
	for _, it := range m.queue.list.items {
		if it.queued {
			n++
		}
	}
	if n == 0 {
		m.setFlash("Your queue is empty", false)
		return nil
	}
	m.confirm("Clear your queue ("+plural(n, "song")+")?", func(m *Model) tea.Cmd {
		var items []listItem
		for _, it := range m.queue.list.items {
			if !it.queued {
				items = append(items, it)
			}
		}
		m.queue.list.setItems(items)
		return m.editQueue("Clear queue", "Queue cleared", daemon.ApiQueueEdit{Op: daemon.QueueClear})
	})
	return nil
}

// playNext puts the selected track (in any list) at the front of the
// queue.
func (m *Model) playNext() tea.Cmd {
	l := m.activeList()
	if l == nil || m.page == pageNowPlaying {
		return nil
	}
	it := l.selected()
	if it == nil || it.kind != kindTrack {
		return nil
	}
	if m.page == pageQueue && it.queued {
		// Already queued: move it to the front.
		return m.editQueue("Play next", "“"+it.title+"” plays next",
			daemon.ApiQueueEdit{Op: daemon.QueueMove, Uid: it.uid, Uri: it.uri, To: 0})
	}
	return m.editQueue("Play next", "“"+it.title+"” plays next",
		daemon.ApiQueueEdit{Op: daemon.QueueInsert, Uri: it.uri, To: 0})
}

// handleQueueKey takes the queue keys on the queue page.
func (m Model) handleQueueKey(key string) (bool, tea.Model, tea.Cmd) {
	if m.page != pageQueue {
		return false, m, nil
	}
	switch key {
	case "x", "delete":
		return true, m, m.removeFromQueue()
	case "U":
		return true, m, m.undoRemoveFromQueue()
	case "K", "shift+up":
		return true, m, m.moveInQueue(-1)
	case "J", "shift+down":
		return true, m, m.moveInQueue(+1)
	case "X":
		return true, m, m.clearQueue()
	}
	return false, m, nil
}
