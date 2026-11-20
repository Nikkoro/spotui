package spclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	librespot "github.com/devgianlu/go-librespot"
)

func TestDjContextFollowsLexiconSession(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/context-resolve/v1/" + djContextUri:
			_, _ = w.Write([]byte(`{"uri":"` + djContextUri + `","metadata":{"lexicon_context_url":"hm://lexicon-session-provider/context-resolve/v2/session?x=1","dj":"yes"},"pages":[{}]}`))
		case "/lexicon-session-provider/context-resolve/v2/session":
			_, _ = w.Write([]byte(`{"pages":[{"tracks":[{"uri":"spotify:track:old"}],"next_page_url":"hm://lexicon-session-provider/context-resolve/v2/session/next"}]}`))
		case "/lexicon-session-provider/context-resolve/v2/session/next":
			_, _ = w.Write([]byte(`{"tracks":[{"uri":"spotify:track:new1"},{"uri":"spotify:track:new2"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL + "/")
	c := &Spclient{
		log:         &librespot.NullLogger{},
		client:      srv.Client(),
		baseUrl:     base,
		accessToken: func(context.Context, bool) (string, error) { return "t", nil },
	}

	spotCtx, err := c.ContextResolve(context.Background(), djContextUri)
	if err != nil {
		t.Fatal(err, paths)
	}
	if spotCtx.Uri != djContextUri || len(spotCtx.Pages) != 1 || len(spotCtx.Pages[0].Tracks) != 2 ||
		spotCtx.Pages[0].Tracks[0].Uri != "spotify:track:new1" {
		t.Fatalf("got %+v", spotCtx)
	}
	if spotCtx.Metadata["dj"] != "yes" {
		t.Fatal("shell metadata not kept")
	}
	for _, p := range paths {
		if p == "/playlist/v2/playlist/37i9dQZF1EYkqdzj48dyYq" {
			t.Fatal("DJ must not go to the playlist service")
		}
	}
}

func TestLexiconUrlGuard(t *testing.T) {
	for u, ok := range map[string]bool{
		"hm://lexicon-session-provider/a":       true,
		"hm://lexicon-session-provider/../x":    false,
		"hm://playlist/v2/playlist/x":           false,
		"https://evil/lexicon-session-provider": false,
	} {
		if isLexiconUrl(u) != ok {
			t.Errorf("%s: want %v", u, ok)
		}
	}
}
