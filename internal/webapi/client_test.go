package webapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(srv *httptest.Server) *Client {
	c := NewClient(TokenFunc(func(context.Context) (string, error) { return "tok", nil }))
	c.base = srv.URL
	return c
}

func TestSearchIsCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Error("missing token")
		}
		_, _ = w.Write([]byte(`{"tracks":{"items":[{"uri":"spotify:track:1","name":"A"}]}}`))
	}))
	defer srv.Close()
	c := testClient(srv)

	for i := 0; i < 3; i++ {
		res, err := c.Search(context.Background(), "m83")
		if err != nil || len(res.Tracks) != 1 {
			t.Fatalf("search: %+v %v", res, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d requests for the same search, want 1", n)
	}
	if _, err := c.Search(context.Background(), "daft punk"); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("a different search must be asked: %d requests", n)
	}
}

func TestBackoffBlocksEveryCall(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "40")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(srv)

	_, err := c.Search(context.Background(), "x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
		t.Fatalf("got %v, want 429", err)
	}
	if apiErr.RetryAfter < 40*time.Second || apiErr.RetryAfter > 45*time.Second {
		t.Fatalf("retry after %s, want 40s plus up to 10%%", apiErr.RetryAfter)
	}

	// Other endpoints wait too, without a request.
	if _, err := c.TopTracks(context.Background()); !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want the backoff error", err)
	}
	if _, err := c.Search(context.Background(), "y"); err == nil {
		t.Fatal("search went through during backoff")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d requests sent, only the first may go out", n)
	}

	// Once the time is up, requests flow again.
	c.mu.Lock()
	c.blockedUntil = time.Now().Add(-time.Second)
	c.mu.Unlock()
	_, _ = c.Search(context.Background(), "z")
	if n := hits.Load(); n != 2 {
		t.Fatalf("no request after the backoff ended (%d)", n)
	}
}

func TestBackoffWithoutRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(srv)
	_, err := c.Search(context.Background(), "x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter < defaultBackoff {
		t.Fatalf("got %v, want the default back-off", err)
	}
}

func TestCacheExpires(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	c := testClient(srv)
	_, _ = c.RecentlyPlayed(context.Background())
	c.mu.Lock()
	for k, e := range c.cache {
		e.at = e.at.Add(-ttlRecent - time.Second)
		c.cache[k] = e
	}
	c.mu.Unlock()
	_, _ = c.RecentlyPlayed(context.Background())
	if n := hits.Load(); n != 2 {
		t.Fatalf("expired entry reused (%d requests)", n)
	}
}
