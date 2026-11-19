package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const baseURL = "https://api.spotify.com/v1"

// Client is a minimal Web API client.
type Client struct {
	tokens TokenSource
	http   *http.Client
}

// NewClient builds a client that authenticates with tokens.
func NewClient(tokens TokenSource) *Client {
	return &Client{tokens: tokens, http: &http.Client{Timeout: 20 * time.Second}}
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

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}

	u := baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
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
	return json.NewDecoder(resp.Body).Decode(out)
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
}

// Search queries tracks, albums, artists and playlists. Development mode apps
// get at most 10 results per type.
func (c *Client) Search(ctx context.Context, query string) (*SearchResult, error) {
	var raw struct {
		Tracks    page[Track]    `json:"tracks"`
		Albums    page[Album]    `json:"albums"`
		Artists   page[Artist]   `json:"artists"`
		Playlists page[Playlist] `json:"playlists"`
	}
	q := url.Values{
		"q":     {query},
		"type":  {"track,album,artist,playlist"},
		"limit": {"10"},
	}
	if err := c.get(ctx, "/search", q, &raw); err != nil {
		return nil, err
	}

	return &SearchResult{
		Tracks:    compact(raw.Tracks.Items),
		Albums:    compact(raw.Albums.Items),
		Artists:   compact(raw.Artists.Items),
		Playlists: compact(raw.Playlists.Items),
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
