package ui

import (
	"fmt"
	"net/url"
	"strings"
)

var playableTypes = map[string]bool{
	"track": true, "album": true, "playlist": true, "artist": true,
	"episode": true, "show": true,
}

// parseSpotifyURI accepts a spotify: URI or an open.spotify.com link and
// returns the canonical spotify:type:id URI.
func parseSpotifyURI(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}

	if strings.HasPrefix(input, "spotify:") {
		parts := strings.Split(input, ":")
		if len(parts) >= 3 && parts[1] != "" && parts[len(parts)-1] != "" {
			return input, nil
		}
		return "", fmt.Errorf("invalid Spotify URI")
	}

	u, err := url.Parse(input)
	if err != nil || !strings.HasSuffix(u.Host, "spotify.com") {
		return "", fmt.Errorf("not a Spotify link or URI")
	}

	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	// Links can carry a locale prefix like /intl-pl/.
	if len(segs) > 0 && strings.HasPrefix(segs[0], "intl-") {
		segs = segs[1:]
	}
	if len(segs) < 2 || !playableTypes[segs[0]] {
		return "", fmt.Errorf("unsupported Spotify link")
	}
	return "spotify:" + segs[0] + ":" + segs[1], nil
}
