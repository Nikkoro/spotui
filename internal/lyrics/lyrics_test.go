package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseLRC(t *testing.T) {
	lines := ParseLRC("[ar:M83]\n[ti:Midnight City]\n[offset:+500]\n" +
		"[00:01.00]first\n" +
		"[00:10.5][00:30.25]chorus\n" +
		"[01:02.123]third\r\n" +
		"untimed line\n" +
		"[00:20]  middle  \n")

	want := []Line{
		{500 * time.Millisecond, "first"},
		{10*time.Second - 0, "chorus"},
		{19*time.Second + 500*time.Millisecond, "middle"},
		{29*time.Second + 750*time.Millisecond, "chorus"},
		{61*time.Second + 623*time.Millisecond, "third"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines: %+v", len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, lines[i], want[i])
		}
	}
}

func TestIndex(t *testing.T) {
	lines := []Line{{time.Second, "a"}, {5 * time.Second, "b"}, {9 * time.Second, "c"}}
	cases := map[time.Duration]int{0: -1, time.Second: 0, 4 * time.Second: 0, 5 * time.Second: 1, time.Minute: 2}
	for pos, want := range cases {
		if got := Index(lines, pos); got != want {
			t.Errorf("Index(%s) = %d, want %d", pos, got, want)
		}
	}
}

func TestClientFallsBackToSearchAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		switch r.URL.Path {
		case "/get":
			http.NotFound(w, r)
		case "/v2/search":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]record{
				{ID: 1, TrackName: "x", Duration: 400, HasSyncedLyrics: true},
				{ID: 2, TrackName: "x", Duration: 243, HasPlainLyrics: true},
				{ID: 3, TrackName: "x", Duration: 244, HasSyncedLyrics: true},
			})
		case "/get/3":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record{ID: 3, SyncedLyrics: "[00:01.00]right"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(t.TempDir(), "test")
	c.base = srv.URL
	q := Query{URI: "spotify:track:1", Track: "x", Artist: "y", Duration: 243 * time.Second}

	l, err := c.Get(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Synced) != 1 || l.Synced[0].Text != "right" {
		t.Fatalf("picked %+v", l)
	}

	before := calls
	if _, err := c.Get(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if calls != before {
		t.Fatal("second lookup should come from the cache")
	}
}

func TestClientCachesMisses(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/v2/search" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient(t.TempDir(), "test")
	c.base = srv.URL
	q := Query{URI: "spotify:track:2", Track: "a", Artist: "b"}
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), q); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	}
	if calls != 2 { // get + search once, then cached
		t.Fatalf("got %d calls, want 2", calls)
	}
}

func TestClientHonoursRetryAfter(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewClient("", "test")
	c.base = srv.URL
	q := Query{Track: "a", Artist: "b"}

	var rl *RateLimitedError
	if _, err := c.Get(context.Background(), q); !errors.As(err, &rl) {
		t.Fatalf("got %v, want RateLimitedError", err)
	}
	if d := time.Until(rl.Until); d < 119*time.Second || d > 157*time.Second {
		t.Fatalf("backoff %s, want 120s plus up to 30%% jitter", d)
	}
	if _, err := c.Get(context.Background(), q); !errors.As(err, &rl) {
		t.Fatalf("got %v, want RateLimitedError", err)
	}
	if calls != 1 {
		t.Fatalf("sent %d requests, nothing may be sent during backoff", calls)
	}
}
