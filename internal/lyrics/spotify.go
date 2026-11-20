package lyrics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SpotifySource fetches a track's lyrics from Spotify (the source its own
// apps show). It returns ErrNotFound when Spotify has none and a
// *RateLimitedError when asked to wait.
type SpotifySource func(ctx context.Context, uri string) (*Lyrics, error)

// spotifyMissTTL: Spotify adds lyrics over time too, so a miss is asked
// again after a day.
const spotifyMissTTL = 24 * time.Hour

type spotifyEntry struct {
	Found  bool      `json:"found"`
	Lyrics *Lyrics   `json:"lyrics,omitempty"`
	At     time.Time `json:"at"`
}

// WithSpotify makes Spotify the first source, with LRCLIB behind it.
func (c *Client) WithSpotify(src SpotifySource) *Client {
	c.spotify = src
	return c
}

// fromSpotify returns Spotify's lyrics for q, cached; ok is false when
// LRCLIB should be asked instead (none, not a track, or Spotify busy).
func (c *Client) fromSpotify(ctx context.Context, q Query) (*Lyrics, bool) {
	if c.spotify == nil || !strings.HasPrefix(q.URI, "spotify:track:") {
		return nil, false
	}
	if e, ok := c.readSpotifyCache(q.URI); ok {
		return e.Lyrics, e.Found && e.Lyrics != nil
	}

	c.mu.Lock()
	busy := time.Now().Before(c.spotifyBackoff)
	c.mu.Unlock()
	if busy {
		return nil, false
	}

	l, err := c.spotify(ctx, q.URI)
	var rl *RateLimitedError
	switch {
	case err == nil && l != nil && !l.Empty():
		c.writeSpotifyCache(q.URI, spotifyEntry{Found: true, Lyrics: l, At: time.Now()})
		return l, true
	case err == nil, errors.Is(err, ErrNotFound):
		c.writeSpotifyCache(q.URI, spotifyEntry{Found: false, At: time.Now()})
	case errors.As(err, &rl):
		c.mu.Lock()
		c.spotifyBackoff = rl.Until
		c.mu.Unlock()
	}
	// Other errors (network, a hiccup) are not remembered: next time asks again.
	return nil, false
}

func (c *Client) spotifyCachePath(uri string) string {
	if c.cacheDir == "" {
		return ""
	}
	sum := sha1.Sum([]byte("spotify\x00" + uri))
	return filepath.Join(c.cacheDir, "spotify", hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readSpotifyCache(uri string) (spotifyEntry, bool) {
	p := c.spotifyCachePath(uri)
	if p == "" {
		return spotifyEntry{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return spotifyEntry{}, false
	}
	var e spotifyEntry
	if json.Unmarshal(b, &e) != nil {
		return spotifyEntry{}, false
	}
	if !e.Found && time.Since(e.At) > spotifyMissTTL {
		return spotifyEntry{}, false
	}
	return e, true
}

func (c *Client) writeSpotifyCache(uri string, e spotifyEntry) {
	p := c.spotifyCachePath(uri)
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	if b, err := json.Marshal(e); err == nil {
		_ = os.WriteFile(p, b, 0o600)
	}
}
