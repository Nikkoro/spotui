package lyrics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Query identifies a track.
type Query struct {
	URI      string
	Track    string
	Artist   string
	Album    string
	Duration time.Duration
}

// ErrNotFound means no lyrics exist for the track.
var ErrNotFound = errors.New("no lyrics found")

// RateLimitedError means LRCLIB asked us to back off.
type RateLimitedError struct{ Until time.Time }

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("lyrics service busy, retrying after %s", time.Until(e.Until).Round(time.Second))
}

// minGap spaces consecutive requests, as LRCLIB asks of API clients.
const minGap = 350 * time.Millisecond

// Client fetches lyrics from LRCLIB (https://lrclib.net), a free, open
// database of synced lyrics, with an on-disk cache including misses.
//
// It follows LRCLIB's requests to API clients: an identifying User-Agent,
// one request at a time with a short gap between them, and honouring
// Retry-After (with jitter) on 429 and 503 without sending anything else
// until it has passed.
type Client struct {
	http     *http.Client
	base     string
	cacheDir string
	agent    string

	mu      sync.Mutex // serialises requests
	lastReq time.Time
	backoff time.Time

	spotify        SpotifySource // asked first when set
	spotifyBackoff time.Time
}

// NewClient caches results under cacheDir (empty disables caching).
func NewClient(cacheDir, version string) *Client {
	return &Client{
		http:     &http.Client{Timeout: 15 * time.Second},
		base:     "https://lrclib.net/api",
		cacheDir: cacheDir,
		agent:    "spotui/" + version + " (https://github.com/Nikkoro/spotui)",
	}
}

type record struct {
	ID              int64   `json:"id"`
	TrackName       string  `json:"trackName"`
	ArtistName      string  `json:"artistName"`
	AlbumName       string  `json:"albumName"`
	Duration        float64 `json:"duration"`
	Instrumental    bool    `json:"instrumental"`
	PlainLyrics     string  `json:"plainLyrics"`
	SyncedLyrics    string  `json:"syncedLyrics"`
	HasSyncedLyrics bool    `json:"hasSyncedLyrics"`
	HasPlainLyrics  bool    `json:"hasPlainLyrics"`
}

func (r *record) hasSynced() bool { return r.SyncedLyrics != "" || r.HasSyncedLyrics }
func (r *record) hasAny() bool {
	return r.hasSynced() || r.PlainLyrics != "" || r.HasPlainLyrics || r.Instrumental
}

type cacheEntry struct {
	Found  bool      `json:"found"`
	Record record    `json:"record"`
	At     time.Time `json:"at"`
}

// Misses are retried after a while, lyrics get added to LRCLIB over time.
const missTTL = 3 * 24 * time.Hour

// Get returns lyrics for q: Spotify's when a Spotify source is set and has
// them, LRCLIB's otherwise.
func (c *Client) Get(ctx context.Context, q Query) (*Lyrics, error) {
	if l, ok := c.fromSpotify(ctx, q); ok {
		return l, nil
	}
	if e, ok := c.readCache(q); ok {
		if !e.Found {
			return nil, ErrNotFound
		}
		return fromRecord(e.Record), nil
	}

	rec, err := c.lookup(ctx, q)
	switch {
	case errors.Is(err, ErrNotFound):
		c.writeCache(q, cacheEntry{Found: false, At: time.Now()})
		return nil, err
	case err != nil:
		return nil, err
	}
	c.writeCache(q, cacheEntry{Found: true, Record: *rec, At: time.Now()})
	return fromRecord(*rec), nil
}

func fromRecord(r record) *Lyrics {
	l := &Lyrics{Instrumental: r.Instrumental, Source: "LRCLIB"}
	if r.SyncedLyrics != "" {
		l.Synced = ParseLRC(r.SyncedLyrics)
	}
	if r.PlainLyrics != "" {
		l.Plain = strings.Split(strings.ReplaceAll(r.PlainLyrics, "\r", ""), "\n")
	}
	return l
}

// lookup tries the exact signature first (cheapest for LRCLIB), then the v2
// search, which lists candidates without lyrics, and fetches only the best
// one by id.
func (c *Client) lookup(ctx context.Context, q Query) (*record, error) {
	params := url.Values{
		"track_name":  {q.Track},
		"artist_name": {q.Artist},
	}
	if q.Album != "" {
		params.Set("album_name", q.Album)
	}
	if secs := int(q.Duration.Round(time.Second).Seconds()); secs >= 1 && secs <= 3600 {
		params.Set("duration", strconv.Itoa(secs))
	}

	var rec record
	err := c.get(ctx, "/get", params, &rec)
	if err == nil && rec.hasAny() {
		return &rec, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	var results []record
	search := url.Values{"track_name": {q.Track}, "artist_name": {q.Artist}}
	if err := c.get(ctx, "/v2/search", search, &results); err != nil {
		return nil, err
	}
	best := pick(results, q.Duration)
	if best == nil {
		return nil, ErrNotFound
	}
	if best.SyncedLyrics != "" || best.PlainLyrics != "" || best.ID == 0 {
		return best, nil
	}

	var full record
	if err := c.get(ctx, "/get/"+strconv.FormatInt(best.ID, 10), nil, &full); err != nil {
		return nil, err
	}
	return &full, nil
}

func pick(results []record, dur time.Duration) *record {
	var best *record
	bestScore := math.Inf(1)
	for i := range results {
		r := &results[i]
		if !r.hasAny() {
			continue
		}
		score := 0.0
		if dur > 0 && r.Duration > 0 {
			diff := math.Abs(r.Duration - dur.Seconds())
			if diff > 10 {
				continue // a different version of the song
			}
			score += diff
		}
		if !r.hasSynced() {
			score += 100 // plain text only as a last resort
		}
		if score < bestScore {
			best, bestScore = r, score
		}
	}
	return best
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if until := c.backoff; time.Now().Before(until) {
		return &RateLimitedError{Until: until}
	}
	if wait := minGap - time.Since(c.lastReq); wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	defer func() { c.lastReq = time.Now() }()

	u := c.base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		c.backoff = time.Now().Add(retryAfter(resp.Header.Get("Retry-After"), resp.StatusCode))
		return &RateLimitedError{Until: c.backoff}
	default:
		return fmt.Errorf("lrclib: %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") {
		return fmt.Errorf("lrclib: unexpected content type %q", ct)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// retryAfter parses Retry-After seconds and adds up to 30% jitter, as
// LRCLIB asks.
func retryAfter(h string, status int) time.Duration {
	d := 60 * time.Second
	if status == http.StatusServiceUnavailable {
		d = time.Second
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	}
	return d + time.Duration(rand.Float64()*0.3*float64(d))
}

func (c *Client) cachePath(q Query) string {
	if c.cacheDir == "" {
		return ""
	}
	key := q.URI
	if key == "" {
		key = q.Artist + "\x00" + q.Track
	}
	sum := sha1.Sum([]byte(key))
	return filepath.Join(c.cacheDir, hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readCache(q Query) (cacheEntry, bool) {
	p := c.cachePath(q)
	if p == "" {
		return cacheEntry{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil {
		return cacheEntry{}, false
	}
	if !e.Found && time.Since(e.At) > missTTL {
		return cacheEntry{}, false
	}
	return e, true
}

func (c *Client) writeCache(q Query, e cacheEntry) {
	p := c.cachePath(q)
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
