package lyrics

import (
	"context"
	"testing"
	"time"
)

func TestSpotifyFirstThenCache(t *testing.T) {
	calls := 0
	c := NewClient(t.TempDir(), "test").WithSpotify(func(ctx context.Context, uri string) (*Lyrics, error) {
		calls++
		// Invented text.
		return &Lyrics{Source: "Spotify", Synced: []Line{{At: time.Second, Text: "a made-up line"}}}, nil
	})
	q := Query{URI: "spotify:track:abc", Track: "x", Artist: "y"}
	for i := 0; i < 2; i++ {
		l, err := c.Get(context.Background(), q)
		if err != nil || l.Source != "Spotify" || len(l.Synced) != 1 {
			t.Fatalf("%+v %v", l, err)
		}
	}
	if calls != 1 {
		t.Fatalf("Spotify asked %d times, want once (cached)", calls)
	}
}

func TestSpotifyBusyFallsBack(t *testing.T) {
	calls := 0
	c := NewClient("", "test").WithSpotify(func(ctx context.Context, uri string) (*Lyrics, error) {
		calls++
		return nil, &RateLimitedError{Until: time.Now().Add(time.Hour)}
	})
	c.base = "http://127.0.0.1:1" // LRCLIB unreachable: the call fails, Spotify was still skipped politely
	q := Query{URI: "spotify:track:abc", Track: "x", Artist: "y"}
	_, _ = c.Get(context.Background(), q)
	_, _ = c.Get(context.Background(), q)
	if calls != 1 {
		t.Fatalf("Spotify asked %d times while it said to wait", calls)
	}
}
