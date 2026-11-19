package ui

import "testing"

func TestParseSpotifyURI(t *testing.T) {
	cases := map[string]string{
		"spotify:track:4uLU6hMCjMI75M1A2tKUQC":                          "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
		"https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC?si=abc":  "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
		"https://open.spotify.com/intl-pl/album/1DFixLWuPkv3KT3TnV35m3": "spotify:album:1DFixLWuPkv3KT3TnV35m3",
		"  https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M  ":  "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
		"spotify:user:someone:collection":                               "spotify:user:someone:collection",
	}
	for in, want := range cases {
		got, err := parseSpotifyURI(in)
		if err != nil || got != want {
			t.Errorf("parseSpotifyURI(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	for _, bad := range []string{"", "hello", "https://example.com/track/x", "https://open.spotify.com/user/x", "spotify:"} {
		if got, err := parseSpotifyURI(bad); err == nil {
			t.Errorf("parseSpotifyURI(%q) = %q, want error", bad, got)
		}
	}
}
