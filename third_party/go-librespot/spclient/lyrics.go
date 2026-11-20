package spclient

// spotui patch, see third_party/PATCHES.md: a track's lyrics from Spotify's
// color-lyrics service, the source its own apps show.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	librespot "github.com/devgianlu/go-librespot"
)

// ErrNoLyrics means Spotify has no lyrics for the track.
var ErrNoLyrics = errors.New("no lyrics")

// LyricsRateLimitedError means Spotify asked to wait until Until.
type LyricsRateLimitedError struct{ Until time.Time }

func (e *LyricsRateLimitedError) Error() string {
	return "lyrics rate limited until " + e.Until.Format(time.RFC3339)
}

// LyricsLine is one line, from StartMs into the track.
type LyricsLine struct {
	StartMs int64
	Words   string
}

// Lyrics of one track. Synced is false for text without timings.
type Lyrics struct {
	Synced   bool
	Lines    []LyricsLine
	Provider string // e.g. "Musixmatch"
	Language string
}

type lyricsJSON struct {
	Lyrics struct {
		SyncType            string `json:"syncType"`
		ProviderDisplayName string `json:"providerDisplayName"`
		Language            string `json:"language"`
		Lines               []struct {
			StartTimeMs string `json:"startTimeMs"`
			Words       string `json:"words"`
		} `json:"lines"`
	} `json:"lyrics"`
}

// Lyrics fetches the lyrics of a track.
func (c *Spclient) Lyrics(ctx context.Context, id librespot.SpotifyId) (*Lyrics, error) {
	if id.Type() != librespot.SpotifyIdTypeTrack {
		return nil, ErrNoLyrics
	}
	header := http.Header{"Accept": []string{"application/json"}}
	resp, err := c.Request(ctx, "GET", "/color-lyrics/v2/track/"+id.Base62(), nil, header, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusNoContent:
		return nil, ErrNoLyrics
	case http.StatusTooManyRequests:
		wait := time.Minute
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return nil, &LyricsRateLimitedError{Until: time.Now().Add(wait)}
	default:
		return nil, fmt.Errorf("lyrics: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseLyrics(body)
}

func parseLyrics(body []byte) (*Lyrics, error) {
	var j lyricsJSON
	if err := json.Unmarshal(body, &j); err != nil {
		return nil, fmt.Errorf("lyrics: %w", err)
	}
	if len(j.Lyrics.Lines) == 0 {
		return nil, ErrNoLyrics
	}
	l := &Lyrics{
		Synced:   j.Lyrics.SyncType == "LINE_SYNCED",
		Provider: j.Lyrics.ProviderDisplayName,
		Language: j.Lyrics.Language,
	}
	for _, line := range j.Lyrics.Lines {
		ms, _ := strconv.ParseInt(line.StartTimeMs, 10, 64)
		l.Lines = append(l.Lines, LyricsLine{StartMs: ms, Words: line.Words})
	}
	return l, nil
}
