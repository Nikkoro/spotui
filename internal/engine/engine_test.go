package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"
)

// fakeDaemon answers requests the way the real player loop does.
func fakeDaemon(t *testing.T, s *localServer, handle func(daemon.ApiRequest) (any, error)) {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case <-done:
				return
			case req := <-s.Receive():
				data, err := handle(req)
				req.Reply(data, err)
			}
		}
	}()
}

func TestRequestRoundTrip(t *testing.T) {
	s := newLocalServer()
	e := &Engine{server: s}
	fakeDaemon(t, s, func(req daemon.ApiRequest) (any, error) {
		switch req.Type {
		case daemon.ApiRequestTypeStatus:
			return &daemon.ApiStatus{Username: "niko", Volume: 42}, nil
		case daemon.ApiRequestTypeToken:
			return daemon.ApiToken{Token: "t"}, nil // value, not pointer: both must decode
		case daemon.ApiRequestTypeSeek:
			if d := req.Data.(daemon.ApiSeek); d.Position != 5000 || !d.Relative {
				return nil, errors.New("bad seek payload")
			}
			return nil, nil
		}
		return nil, errors.New("unexpected " + string(req.Type))
	})

	ctx := context.Background()
	st, err := e.Status(ctx)
	if err != nil || st.Username != "niko" || st.Volume != 42 {
		t.Fatalf("status: %+v %v", st, err)
	}
	if tok, err := e.Token(ctx); err != nil || tok != "t" {
		t.Fatalf("token: %q %v", tok, err)
	}
	if err := e.Seek(ctx, 5000, true); err != nil {
		t.Fatal(err)
	}
}

func TestRequestTimesOutWhenNobodyListens(t *testing.T) {
	e := &Engine{server: newLocalServer()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := e.Status(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want deadline exceeded", err)
	}
}

func TestEventsNeverBlockTheDaemon(t *testing.T) {
	s := newLocalServer()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			s.Emit(&daemon.ApiEvent{Type: daemon.ApiEventTypePlaying})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked with nobody reading")
	}
	_ = s.Close()
	s.Emit(&daemon.ApiEvent{Type: daemon.ApiEventTypePlaying}) // after close: no panic
}

func TestAuthEvents(t *testing.T) {
	s := newLocalServer()
	s.SetAuthCode(&daemon.ApiDeviceAuth{Code: "ABCD"})
	s.SetAuthCode(nil)
	first := (<-s.events).(AuthEvent)
	second := (<-s.events).(AuthEvent)
	if first.Pending == nil || first.Pending.Code != "ABCD" || second.Pending != nil {
		t.Fatalf("got %+v then %+v", first, second)
	}
}
