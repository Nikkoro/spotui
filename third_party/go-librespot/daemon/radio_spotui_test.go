package daemon

import (
	"context"
	"errors"
	"testing"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
)

func TestRadioStation(t *testing.T) {
	ctx := context.Background()
	noTracks := func(context.Context) []string { return nil }
	own := func(context.Context, string) (bool, error) { return true, nil }
	none := func(context.Context, string) (bool, error) { return false, errors.New("404") }
	var gotRecent []string
	autoplay := func(_ context.Context, seed string, recent []string) (string, error) {
		gotRecent = recent
		return "spotify:station:made:1", nil
	}

	if uri, err := radioStation(ctx, ApiRadio{Seed: "spotify:artist:a1"}, own, autoplay, noTracks); err != nil || uri != "spotify:station:artist:a1" {
		t.Fatalf("own station: %q %v", uri, err)
	}
	uri, err := radioStation(ctx, ApiRadio{Seed: "spotify:track:t1"}, none, autoplay, noTracks)
	if err != nil || uri != "spotify:station:made:1" || len(gotRecent) != 1 || gotRecent[0] != "spotify:track:t1" {
		t.Fatalf("autoplay: %q %v %v", uri, err, gotRecent)
	}
	failing := func(context.Context, string, []string) (string, error) { return "", errors.New("nope") }
	if _, err := radioStation(ctx, ApiRadio{Seed: "spotify:album:x"}, none, failing, noTracks); err == nil {
		t.Fatal("no error when nothing has a station")
	}
	// An album with neither: the station of its first song.
	songsOnly := func(_ context.Context, uri string) (bool, error) {
		return uri == "spotify:station:track:s1", nil
	}
	firstSongs := func(context.Context) []string { return []string{"spotify:track:s1", "spotify:track:s2"} }
	if uri, err := radioStation(ctx, ApiRadio{Seed: "spotify:album:x"}, songsOnly, failing, firstSongs); err != nil || uri != "spotify:station:track:s1" {
		t.Fatalf("song fallback: %q %v", uri, err)
	}
}

func TestPrependContextTrack(t *testing.T) {
	ctx := &connectpb.Context{Pages: []*connectpb.ContextPage{{Tracks: []*connectpb.ContextTrack{{Uri: "spotify:track:b"}}}}}
	prependContextTrack(ctx, "spotify:track:a")
	if got := ctx.Pages[0].Tracks; len(got) != 2 || got[0].Uri != "spotify:track:a" {
		t.Fatalf("not first: %v", got)
	}
	prependContextTrack(ctx, "spotify:track:b") // already there: moved to the front
	if got := ctx.Pages[0].Tracks; len(got) != 2 || got[0].Uri != "spotify:track:b" || got[1].Uri != "spotify:track:a" {
		t.Fatalf("moved: %v", got)
	}
	lazy := &connectpb.Context{Pages: []*connectpb.ContextPage{{PageUrl: "hm://next"}}}
	prependContextTrack(lazy, "spotify:track:a")
	if len(lazy.Pages) != 2 || lazy.Pages[0].Tracks[0].Uri != "spotify:track:a" || lazy.Pages[1].PageUrl != "hm://next" {
		t.Fatalf("lazy: %v", lazy.Pages)
	}
}
