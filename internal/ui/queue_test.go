package ui

import (
	"strings"
	"testing"
)

func queueModel(t *testing.T) Model {
	m := remoteModel(t)
	m.page = pageQueue
	m.queue.list.setItems([]listItem{
		{kind: kindHeader, title: "Now playing"},
		{kind: kindTrack, title: "Playing", uri: "spotify:track:p"},
		{kind: kindHeader, title: "In your queue"},
		{kind: kindTrack, title: "Q1", uri: "spotify:track:q1", uid: "q1", queued: true},
		{kind: kindTrack, title: "Q2", uri: "spotify:track:q2", uid: "q2", queued: true},
		{kind: kindHeader, title: "Next up"},
		{kind: kindTrack, title: "N1", uri: "spotify:track:n1"},
	})
	return m
}

func queueTitles(l listModel) string {
	var s []string
	for _, it := range l.items {
		if it.kind == kindTrack {
			s = append(s, it.title)
		}
	}
	return strings.Join(s, " ")
}

func selectTitle(t *testing.T, l *listModel, title string) {
	for i, idx := range l.visible {
		if l.items[idx].title == title {
			l.cursor = i
			return
		}
	}
	t.Fatalf("no row %q", title)
}

func TestQueuePos(t *testing.T) {
	m := queueModel(t)
	l := &m.queue.list
	for idx, want := range map[int]int{1: -1, 3: 0, 4: 1, 6: -1} {
		if got := queuePos(l, idx); got != want {
			t.Errorf("row %d: %d, want %d", idx, got, want)
		}
	}
}

func TestQueueRemoveUndoMove(t *testing.T) {
	m := queueModel(t)
	selectTitle(t, &m.queue.list, "Q1")
	m = press(m, "x")
	if got := queueTitles(m.queue.list); got != "Playing Q2 N1" {
		t.Fatalf("after x: %q", got)
	}
	if m.queueUndo == nil || m.queueUndo.index != 0 || m.queueUndo.uri != "spotify:track:q1" {
		t.Fatalf("undo: %+v", m.queueUndo)
	}
	if m.undoRemoveFromQueue() == nil || m.queueUndo != nil {
		t.Fatal("U did nothing")
	}

	m = queueModel(t)
	selectTitle(t, &m.queue.list, "Q1")
	m = press(m, "J")
	if got := queueTitles(m.queue.list); got != "Playing Q2 Q1 N1" {
		t.Fatalf("after J: %q", got)
	}
	m = press(m, "J") // the end of the queue: Next up is not the queue's
	if got := queueTitles(m.queue.list); got != "Playing Q2 Q1 N1" {
		t.Fatalf("moved out of the queue: %q", got)
	}
}

func TestQueueRefusesContextTracks(t *testing.T) {
	m := queueModel(t)
	selectTitle(t, &m.queue.list, "N1")
	m = press(m, "x")
	if got := queueTitles(m.queue.list); got != "Playing Q1 Q2 N1" || !m.flashErr {
		t.Fatalf("removed a context track: %q", got)
	}
}

func TestClearQueueAsksFirst(t *testing.T) {
	m := queueModel(t)
	m = press(m, "X")
	if !m.asking.active || m.asking.label != "Clear your queue (2 songs)?" {
		t.Fatalf("no question: %+v", m.asking.label)
	}
	next, _ := m.updateAsk(keyMsg("y"))
	m = next.(Model)
	if got := queueTitles(m.queue.list); got != "Playing N1" {
		t.Fatalf("after clearing: %q", got)
	}
}
