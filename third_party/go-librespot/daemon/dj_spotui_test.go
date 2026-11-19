//go:build test_unit

package daemon

import (
	"testing"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
)

func TestDjRequestedMatchesOnlyNewRequests(t *testing.T) {
	a := &djPending{current: "r1"}
	cases := map[string]bool{"": false, "r1": false, "r2": true}
	for id, want := range cases {
		tr := &connectpb.ContextTrack{Metadata: map[string]string{}}
		if id != "" {
			tr.Metadata[djRequestIdKey] = id
		}
		if got := a.requested(tr); got != want {
			t.Errorf("id %q: got %v want %v", id, got, want)
		}
	}
}
