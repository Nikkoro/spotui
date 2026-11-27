package daemon

import (
	"errors"
	"strings"
	"testing"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
)

func queueOf(uids ...string) []*connectpb.ContextTrack {
	var q []*connectpb.ContextTrack
	for _, u := range uids {
		q = append(q, &connectpb.ContextTrack{Uid: u, Uri: "spotify:track:" + u})
	}
	return q
}

func uids(q []*connectpb.ContextTrack) string {
	var s []string
	for _, t := range q {
		s = append(s, t.Uid)
	}
	return strings.Join(s, " ")
}

func TestEditQueue(t *testing.T) {
	for _, c := range []struct {
		edit ApiQueueEdit
		want string
	}{
		{ApiQueueEdit{Op: QueueRemove, Uid: "b"}, "a c d"},
		{ApiQueueEdit{Op: QueueRemove, Uri: "spotify:track:d"}, "a b c"},
		{ApiQueueEdit{Op: QueueMove, Uid: "a", To: 2}, "b c a d"},
		{ApiQueueEdit{Op: QueueMove, Uid: "d", To: 0}, "d a b c"},
		{ApiQueueEdit{Op: QueueMove, Uid: "b", To: 99}, "a c d b"},
		{ApiQueueEdit{Op: QueueInsert, Uri: "spotify:track:n"}, "n a b c d"},
		{ApiQueueEdit{Op: QueueInsert, Uri: "spotify:track:n", To: 2}, "a b n c d"},
		{ApiQueueEdit{Op: QueueInsert, Uri: "spotify:track:n", To: 9}, "a b c d n"},
		{ApiQueueEdit{Op: QueueClear}, ""},
	} {
		q := queueOf("a", "b", "c", "d")
		var added *connectpb.ContextTrack
		if c.edit.Op == QueueInsert {
			added = &connectpb.ContextTrack{Uid: "n", Uri: c.edit.Uri}
		}
		got, err := editQueue(q, c.edit, added)
		if err != nil || uids(got) != c.want {
			t.Errorf("%+v: got %q %v, want %q", c.edit, uids(got), err, c.want)
		}
		if uids(q) != "a b c d" {
			t.Errorf("%+v changed its input: %q", c.edit, uids(q))
		}
	}
	if _, err := editQueue(queueOf("a"), ApiQueueEdit{Op: QueueRemove, Uid: "x"}, nil); !errors.Is(err, ErrNotInQueue) {
		t.Fatalf("missing entry: %v", err)
	}
}
