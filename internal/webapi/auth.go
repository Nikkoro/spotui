// Package webapi talks to the Spotify Web API for the things the Connect
// protocol does not offer, such as search.
package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// RedirectURL must be registered in the Spotify developer dashboard. It is
// the same one spotify_player uses, so existing apps already have it.
const RedirectURL = "http://127.0.0.1:8989/login"

var scopes = []string{
	"user-read-private",
	"user-library-read",
	"user-library-modify",
	"user-follow-read",
	"user-top-read",
	"user-read-recently-played",
	"playlist-read-private",
	"playlist-read-collaborative",
	"playlist-modify-private",
	"playlist-modify-public",
}

// ErrNotAuthorized means the user has not approved the app yet.
var ErrNotAuthorized = errors.New("web API not authorized")

// TokenSource returns a bearer token for Web API calls.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// TokenFunc adapts a function, e.g. the librespot session token.
type TokenFunc func(ctx context.Context) (string, error)

// Token implements TokenSource.
func (f TokenFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

// PKCEAuth authorizes with the user's own client id (authorization code with
// PKCE, no client secret) and keeps the refresh token on disk.
type PKCEAuth struct {
	conf *oauth2.Config
	path string

	mu  sync.Mutex
	tok *oauth2.Token
	src oauth2.TokenSource
}

// NewPKCEAuth loads a stored token, if any, from path.
func NewPKCEAuth(clientID, path string) *PKCEAuth {
	a := &PKCEAuth{
		conf: &oauth2.Config{
			ClientID:    clientID,
			RedirectURL: RedirectURL,
			Scopes:      scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://accounts.spotify.com/authorize",
				TokenURL:  "https://accounts.spotify.com/api/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		path: path,
	}

	if b, err := os.ReadFile(path); err == nil {
		var tok oauth2.Token
		if json.Unmarshal(b, &tok) == nil && tok.RefreshToken != "" {
			a.setToken(&tok)
		}
	}
	return a
}

func (a *PKCEAuth) setToken(tok *oauth2.Token) {
	a.tok = tok
	a.src = oauth2.ReuseTokenSource(tok, a.conf.TokenSource(context.Background(), tok))
}

// Authorized reports whether a refresh token is available.
func (a *PKCEAuth) Authorized() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.src != nil
}

// Token implements TokenSource, refreshing and persisting as needed.
func (a *PKCEAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.src == nil {
		return "", ErrNotAuthorized
	}
	tok, err := a.src.Token()
	if err != nil {
		return "", fmt.Errorf("refreshing web API token: %w", err)
	}
	if a.tok == nil || tok.AccessToken != a.tok.AccessToken {
		if tok.RefreshToken == "" && a.tok != nil {
			tok.RefreshToken = a.tok.RefreshToken
		}
		a.tok = tok
		a.save(tok)
	}
	return tok.AccessToken, nil
}

func (a *PKCEAuth) save(tok *oauth2.Token) {
	b, err := json.Marshal(tok)
	if err != nil {
		return
	}
	_ = os.WriteFile(a.path, b, 0o600)
}

// Authorize starts the browser flow: it returns the URL the user must open
// and a channel that yields nil once the redirect arrived and the token was
// stored, or an error. The local callback server stops on its own.
func (a *PKCEAuth) Authorize(ctx context.Context) (string, <-chan error, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:8989")
	if err != nil {
		return "", nil, fmt.Errorf("listening for the login redirect on 127.0.0.1:8989: %w", err)
	}

	verifier := oauth2.GenerateVerifier()
	state := oauth2.GenerateVerifier()
	authURL := a.conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	done := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	var once sync.Once
	finish := func(err error) {
		once.Do(func() {
			done <- err
			go func() { _ = srv.Shutdown(context.Background()) }()
		})
	}

	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			writePage(w, "Authorization failed: "+e)
			finish(fmt.Errorf("authorization denied: %s", e))
			return
		}

		xctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		tok, err := a.conf.Exchange(xctx, q.Get("code"), oauth2.VerifierOption(verifier))
		if err != nil {
			writePage(w, "Could not finish login: "+err.Error())
			finish(fmt.Errorf("exchanging code: %w", err))
			return
		}

		a.mu.Lock()
		a.setToken(tok)
		a.save(tok)
		a.mu.Unlock()

		writePage(w, "spotui is connected. You can close this tab.")
		finish(nil)
	})

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			finish(err)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			finish(ctx.Err())
		case <-time.After(10 * time.Minute):
			finish(errors.New("login timed out"))
		}
	}()

	return authURL, done, nil
}

func writePage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>spotui</title>
<body style="background:#121212;color:#eee;font:16px system-ui;display:grid;place-items:center;height:100vh;margin:0">
<div style="text-align:center"><div style="font-size:48px;color:#1DB954">♫</div><p>%s</p></div></body>`, htmlEscape(msg))
}

func htmlEscape(s string) string {
	r := make([]rune, 0, len(s))
	for _, c := range s {
		switch c {
		case '<', '>', '&', '"':
			r = append(r, []rune(fmt.Sprintf("&#%d;", c))...)
		default:
			r = append(r, c)
		}
	}
	return string(r)
}
