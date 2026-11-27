package spclient

// spotui patch, see third_party/PATCHES.md: the radio playlist the Spotify
// apps make of a song, artist, album or playlist (inspiredby-mix, as
// librespot's get_radio_for_track).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// RadioPlaylist returns the URI of seed's radio playlist ("… Radio").
func (c *Spclient) RadioPlaylist(ctx context.Context, seed string) (string, error) {
	query := url.Values{"response-format": {"json"}}
	resp, err := c.Request(ctx, "GET", "/inspiredby-mix/v2/seed_to_playlist/"+seed, query, nil, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", &StatusError{Op: "radio playlist", StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed reading response body: %w", err)
	}
	return parseRadioPlaylist(body)
}

// parseRadioPlaylist reads {"mediaItems":[{"uri":"spotify:playlist:…"}]}.
func parseRadioPlaylist(body []byte) (string, error) {
	var r struct {
		MediaItems []struct {
			Uri string `json:"uri"`
		} `json:"mediaItems"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("failed parsing radio playlist: %w", err)
	}
	for _, it := range r.MediaItems {
		if strings.HasPrefix(it.Uri, "spotify:playlist:") {
			return it.Uri, nil
		}
	}
	return "", fmt.Errorf("radio playlist answer names no playlist")
}
