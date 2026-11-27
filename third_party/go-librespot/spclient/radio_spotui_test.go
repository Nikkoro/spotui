package spclient

import "testing"

func TestParseRadioPlaylist(t *testing.T) {
	uri, err := parseRadioPlaylist([]byte(`{"total":1,"mediaItems":[{"uri":"spotify:playlist:37i9dQZF1E8abc"}]}`))
	if err != nil || uri != "spotify:playlist:37i9dQZF1E8abc" {
		t.Fatalf("got %q %v", uri, err)
	}
	if _, err := parseRadioPlaylist([]byte(`{"mediaItems":[]}`)); err == nil {
		t.Fatal("empty answer")
	}
}
