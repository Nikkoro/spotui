package daemon

// spotui patch, see third_party/PATCHES.md: editing the user's queue (take
// an entry out, move it, play a track next, clear it). The Web API can only
// append; spotui is the player, so it changes its own queue, the way a
// set_queue from the Spotify app does, and Connect shows the result to
// every device.

import (
	"context"
	"errors"
	"fmt"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
	"github.com/devgianlu/go-librespot/tracks"
)

const ApiRequestTypeQueueEdit ApiRequestType = "queue_edit"

// Queue edits.
const (
	QueueRemove = "remove" // take the entry Uid (or the first Uri) out
	QueueMove   = "move"   // move the entry Uid (or Uri) to index To
	QueueInsert = "insert" // put Uri at index To (0: it plays next)
	QueueClear  = "clear"  // empty the queue
)

// ApiQueueEdit changes the queue: what Op does is in its constant.
type ApiQueueEdit struct {
	Op       string
	Uid, Uri string
	To       int
}

// ErrNotInQueue reports an entry that is not (or no longer) in the user's
// queue: a context's tracks are not the queue's to change.
var ErrNotInQueue = errors.New("not in the queue")

func (p *AppPlayer) handleQueueEdit(req ApiRequest) (any, error) {
	data := req.Data.(ApiQueueEdit)
	var added *connectpb.ContextTrack
	switch data.Op {
	case QueueInsert:
		if data.Uri == "" {
			return nil, ErrBadRequest
		}
		p.state.queueID++
		added = &connectpb.ContextTrack{
			Uri:      data.Uri,
			Uid:      fmt.Sprintf("q%d", p.state.queueID),
			Metadata: map[string]string{"is_queued": "true"},
		}
	case QueueRemove, QueueMove, QueueClear:
	default:
		return nil, ErrBadRequest
	}

	reply := apiReply(req)
	p.listJob("edit queue", classMutate, nil,
		func(_ context.Context, list *tracks.List) error {
			queue, err := editQueue(list.UpcomingQueue(), data, added)
			if err != nil {
				return err
			}
			// SetQueue keeps the entries marked queued, up to the first
			// that is not.
			for _, t := range queue {
				if t.Metadata == nil {
					t.Metadata = map[string]string{}
				}
				t.Metadata["is_queued"] = "true"
			}
			list.SetQueue(nil, queue)
			return nil
		},
		func(p *AppPlayer, snap *tracks.Snapshot, err error) {
			if err != nil {
				reply.done(nil, err)
				return
			}
			p.publishUpcoming(snap)
			// What plays next may have changed: a stream prefetched under the
			// old plan must not be switched or faded into.
			p.invalidateUpcoming()
			p.schedulePrefetchNext()
			p.updateState()
			reply.done(nil, nil)
		})
	return nil, errReplyDeferred
}

// editQueue applies an edit to the upcoming queue and returns the new one.
func editQueue(queue []*connectpb.ContextTrack, data ApiQueueEdit, added *connectpb.ContextTrack) ([]*connectpb.ContextTrack, error) {
	find := func() int {
		for i, t := range queue {
			if (data.Uid != "" && t.Uid == data.Uid) || (data.Uid == "" && t.Uri == data.Uri) {
				return i
			}
		}
		return -1
	}
	switch data.Op {
	case QueueClear:
		return nil, nil
	case QueueInsert:
		to := min(max(data.To, 0), len(queue))
		return append(queue[:to:to], append([]*connectpb.ContextTrack{added}, queue[to:]...)...), nil
	case QueueRemove:
		i := find()
		if i < 0 {
			return nil, ErrNotInQueue
		}
		return append(queue[:i:i], queue[i+1:]...), nil
	case QueueMove:
		i := find()
		if i < 0 {
			return nil, ErrNotInQueue
		}
		to := min(max(data.To, 0), len(queue)-1)
		t := queue[i]
		rest := append(queue[:i:i], queue[i+1:]...)
		return append(rest[:to:to], append([]*connectpb.ContextTrack{t}, rest[to:]...)...), nil
	}
	return nil, ErrBadRequest
}
