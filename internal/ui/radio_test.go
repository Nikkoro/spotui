package ui

import (
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"
)

func TestRadioSeed(t *testing.T) {
	m := remoteModel(t)
	m.track = &daemon.ApiTrack{Uri: "spotify:track:playing", Name: "Playing"}
	if uri, _ := m.radioSeed(); uri != "spotify:track:playing" {
		t.Fatalf("now playing: %q", uri)
	}
	m.page = pageLibrary
	m.lib.tab = libArtists
	m.lib.lists[libArtists].setItems([]listItem{{kind: kindArtist, title: "Someone", uri: "spotify:artist:a1"}})
	if uri, title := m.radioSeed(); uri != "spotify:artist:a1" || title != "Someone" {
		t.Fatalf("selected: %q", uri)
	}
	m.lib.lists[libArtists].setItems([]listItem{{kind: kindShow, title: "Show", uri: "spotify:show:s1"}})
	if uri, _ := m.radioSeed(); uri != "spotify:track:playing" {
		t.Fatalf("a show has no radio, the playing song does: %q", uri)
	}
	m.track = &daemon.ApiTrack{Uri: "spotify:episode:e1"}
	if uri, _ := m.radioSeed(); uri != "" {
		t.Fatalf("episode: %q", uri)
	}
}

func TestRadioUsesItsCache(t *testing.T) {
	m := remoteModel(t)
	m.radios["spotify:album:x"] = radioEntry{station: "spotify:station:album:x", at: time.Now()}
	// A cached station plays at once; a stale one is looked up again.
	if cmd := m.startRadio("spotify:album:x", "X"); cmd == nil || m.flash != "Radio: “X”" {
		t.Fatalf("cached: %q", m.flash)
	}
	m.radios["spotify:album:x"] = radioEntry{station: "spotify:station:album:x", at: time.Now().Add(-2 * radioCacheTTL)}
	if cmd := m.startRadio("spotify:album:x", "X"); cmd == nil || m.flash != "Starting the radio of “X”…" {
		t.Fatalf("stale: %q", m.flash)
	}
	if m.startRadio("spotify:show:s", "S") != nil || !m.flashErr {
		t.Fatal("radio of a show")
	}
}
