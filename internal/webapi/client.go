package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const baseURL = "https://api.spotify.com/v1"

// Client is a minimal Web API client.
type Client struct {
	tokens TokenSource
	http   *http.Client
	base   string

	mu           sync.Mutex
	blockedUntil time.Time
	cache        map[string]cached
}

type cached struct {
	body []byte
	at   time.Time
}

// How long answers are reused. Spotify rate-limits per app over a rolling
// window, so the same question is not asked again within these.
const (
	ttlSearch = 10 * time.Minute
	ttlRecent = 2 * time.Minute
	ttlTop    = time.Hour

	maxCached = 64
	// Used when a 429 comes without Retry-After.
	defaultBackoff = 30 * time.Second
)

// NewClient builds a client that authenticates with tokens.
func NewClient(tokens TokenSource) *Client {
	return &Client{tokens: tokens, http: &http.Client{Timeout: 20 * time.Second}, base: baseURL, cache: map[string]cached{}}
}

// APIError is a non-2xx answer.
type APIError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Status == http.StatusTooManyRequests && e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited, retry in %s", e.RetryAfter.Round(time.Second))
	}
	if e.Message != "" {
		return fmt.Sprintf("web API %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("web API %d", e.Status)
}

// get fetches path, reusing an answer younger than ttl, and sends nothing
// while Spotify has asked us to back off.
func (c *Client) get(ctx context.Context, path string, query url.Values, ttl time.Duration, out any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	c.mu.Lock()
	if e, ok := c.cache[u]; ok && time.Since(e.at) < ttl {
		c.mu.Unlock()
		return json.Unmarshal(e.body, out)
	}
	if wait := time.Until(c.blockedUntil); wait > 0 {
		c.mu.Unlock()
		return &APIError{Status: http.StatusTooManyRequests, RetryAfter: wait}
	}
	c.mu.Unlock()

	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		apiErr := &APIError{Status: resp.StatusCode}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			apiErr.RetryAfter = time.Duration(s) * time.Second
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			// Back off for everything, not just this call, plus a little
			// jitter so retries do not land all at once.
			wait := apiErr.RetryAfter
			if wait <= 0 {
				wait = defaultBackoff
			}
			wait += time.Duration(rand.Float64() * 0.1 * float64(wait))
			apiErr.RetryAfter = wait
			c.mu.Lock()
			if until := time.Now().Add(wait); until.After(c.blockedUntil) {
				c.blockedUntil = until
			}
			c.mu.Unlock()
		}
		var body struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)); json.Unmarshal(b, &body) == nil {
			apiErr.Message = body.Error.Message
		}
		return apiErr
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	c.mu.Lock()
	if len(c.cache) >= maxCached {
		c.evictOldestLocked()
	}
	c.cache[u] = cached{body: body, at: time.Now()}
	c.mu.Unlock()
	return nil
}

func (c *Client) evictOldestLocked() {
	var oldest string
	var at time.Time
	for k, e := range c.cache {
		if oldest == "" || e.at.Before(at) {
			oldest, at = k, e.at
		}
	}
	delete(c.cache, oldest)
}

// Image is a cover or avatar.
type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Named is the common shape of a simplified object.
type Named struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// Track is a simplified track.
type Track struct {
	Named
	DurationMs int     `json:"duration_ms"`
	Explicit   bool    `json:"explicit"`
	Artists    []Named `json:"artists"`
	Album      Album   `json:"album"`
}

// Album is a simplified album.
type Album struct {
	Named
	AlbumType   string  `json:"album_type"`
	ReleaseDate string  `json:"release_date"`
	TotalTracks int     `json:"total_tracks"`
	Artists     []Named `json:"artists"`
	Images      []Image `json:"images"`
}

// Artist is a simplified artist.
type Artist struct {
	Named
	Genres []string `json:"genres"`
	Images []Image  `json:"images"`
}

// Playlist is a simplified playlist.
type Playlist struct {
	Named
	Description string  `json:"description"`
	Images      []Image `json:"images"`
	Owner       struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Tracks struct {
		Total int `json:"total"`
	} `json:"tracks"`
}

type page[T any] struct {
	Items []*T `json:"items"` // entries can be null (e.g. removed playlists)
	Total int  `json:"total"`
}

// SearchResult groups results by type.
type SearchResult struct {
	Tracks    []Track
	Albums    []Album
	Artists   []Artist
	Playlists []Playlist
	Shows     []Show
	Episodes  []Episode
}

// Search queries tracks, albums, artists, playlists, shows and episodes. Development mode apps
// get at most 10 results per type.
func (c *Client) Search(ctx context.Context, query string) (*SearchResult, error) {
	var raw struct {
		Tracks    page[Track]    `json:"tracks"`
		Albums    page[Album]    `json:"albums"`
		Artists   page[Artist]   `json:"artists"`
		Playlists page[Playlist] `json:"playlists"`
		Shows     page[Show]     `json:"shows"`
		Episodes  page[Episode]  `json:"episodes"`
	}
	q := url.Values{
		"q":     {query},
		"type":  {"track,album,artist,playlist,show,episode"},
		"limit": {"10"},
	}
	if err := c.get(ctx, "/search", q, ttlSearch, &raw); err != nil {
		return nil, err
	}

	return &SearchResult{
		Tracks:    compact(raw.Tracks.Items),
		Albums:    compact(raw.Albums.Items),
		Artists:   compact(raw.Artists.Items),
		Playlists: compact(raw.Playlists.Items),
		Shows:     compact(raw.Shows.Items),
		Episodes:  compact(raw.Episodes.Items),
	}, nil
}

func compact[T any](in []*T) []T {
	out := make([]T, 0, len(in))
	for _, v := range in {
		if v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// PlayHistory is one recently played track and where it was played from.
type PlayHistory struct {
	Track    Track  `json:"track"`
	PlayedAt string `json:"played_at"`
	Context  *struct {
		URI string `json:"uri"`
	} `json:"context"`
}

// RecentlyPlayed returns the last tracks the account played.
func (c *Client) RecentlyPlayed(ctx context.Context) ([]PlayHistory, error) {
	var raw struct {
		Items []PlayHistory `json:"items"`
	}
	if err := c.get(ctx, "/me/player/recently-played", url.Values{"limit": {"50"}}, ttlRecent, &raw); err != nil {
		return nil, err
	}
	return raw.Items, nil
}

// TopTracks returns the account's most played tracks of the last weeks.
func (c *Client) TopTracks(ctx context.Context) ([]Track, error) {
	var raw page[Track]
	if err := c.get(ctx, "/me/top/tracks", url.Values{"limit": {"50"}, "time_range": {"short_term"}}, ttlTop, &raw); err != nil {
		return nil, err
	}
	return compact(raw.Items), nil
}

// TopArtists returns the account's most played artists of the last weeks.
func (c *Client) TopArtists(ctx context.Context) ([]Artist, error) {
	var raw page[Artist]
	if err := c.get(ctx, "/me/top/artists", url.Values{"limit": {"50"}, "time_range": {"short_term"}}, ttlTop, &raw); err != nil {
		return nil, err
	}
	return compact(raw.Items), nil
}
