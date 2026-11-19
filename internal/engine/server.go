package engine

import (
	"sync"

	"github.com/devgianlu/go-librespot/daemon"
)

// localServer implements daemon.ApiServer in-process: instead of exposing the
// daemon over HTTP it hands requests over a channel and forwards events to
// the UI.
type localServer struct {
	requests chan daemon.ApiRequest
	events   chan Event

	closeOnce sync.Once
	closed    chan struct{}
}

func newLocalServer() *localServer {
	return &localServer{
		requests: make(chan daemon.ApiRequest),
		events:   make(chan Event, 256),
		closed:   make(chan struct{}),
	}
}

func (s *localServer) Emit(ev *daemon.ApiEvent) {
	if ev == nil {
		return
	}
	s.push(PlayerEvent{Type: ev.Type, Data: ev.Data})
}

func (s *localServer) Receive() <-chan daemon.ApiRequest {
	return s.requests
}

func (s *localServer) SetAuthCode(auth *daemon.ApiDeviceAuth) {
	if auth == nil {
		s.push(AuthEvent{})
		return
	}
	a := *auth
	s.push(AuthEvent{Pending: &a})
}

func (s *localServer) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

// push delivers an event without ever blocking the daemon. If the UI falls
// far behind, the oldest events are the least useful, so the new one is
// dropped rather than stalling playback.
func (s *localServer) push(ev Event) {
	select {
	case <-s.closed:
	case s.events <- ev:
	default:
	}
}
