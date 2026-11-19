package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/devgianlu/go-librespot/daemon"
)

const libraryPageSize = 500

// decode accepts either T or *T from a daemon reply.
func decode[T any](resp any, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	switch v := resp.(type) {
	case *T:
		return v, nil
	case T:
		return &v, nil
	case nil:
		return nil, fmt.Errorf("empty response")
	default:
		return nil, fmt.Errorf("unexpected response %T", resp)
	}
}

func (e *Engine) libraryPage(ctx context.Context, typ daemon.ApiRequestType, offset int) (any, error) {
	return e.request(ctx, typ, daemon.ApiRequestDataLibraryPage{Offset: offset, Limit: libraryPageSize})
}

// Playlists returns the whole playlist rootlist.
func (e *Engine) Playlists(ctx context.Context) ([]daemon.ApiLibraryPlaylist, error) {
	var all []daemon.ApiLibraryPlaylist
	for {
		page, err := decode[daemon.ApiLibraryPlaylists](e.libraryPage(ctx, daemon.ApiRequestTypeLibraryPlaylists, len(all)))
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.Total {
			return all, nil
		}
	}
}

// Albums returns every saved album.
func (e *Engine) Albums(ctx context.Context) ([]daemon.ApiLibraryAlbum, error) {
	var all []daemon.ApiLibraryAlbum
	for {
		page, err := decode[daemon.ApiLibraryAlbums](e.libraryPage(ctx, daemon.ApiRequestTypeLibraryAlbums, len(all)))
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.Total {
			return all, nil
		}
	}
}

// Artists returns every followed artist.
func (e *Engine) Artists(ctx context.Context) ([]daemon.ApiLibraryArtist, error) {
	var all []daemon.ApiLibraryArtist
	for {
		page, err := decode[daemon.ApiLibraryArtists](e.libraryPage(ctx, daemon.ApiRequestTypeLibraryArtists, len(all)))
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.Total {
			return all, nil
		}
	}
}

// ContextTracks lists the tracks of a playlist, album, artist or collection.
// The daemon enumerates large contexts in the background; this returns what
// is known so far, check Ready and Cached against Length.
func (e *Engine) ContextTracks(ctx context.Context, uri string) (*daemon.ApiContextTracks, error) {
	return decode[daemon.ApiContextTracks](e.request(ctx, daemon.ApiRequestTypeContextTracks, daemon.ApiRequestDataContextTracks{Uri: uri}))
}

// WaitContextTracks polls ContextTracks until the listing is complete or
// timeout passes, reporting every intermediate result through progress.
func (e *Engine) WaitContextTracks(ctx context.Context, uri string, timeout time.Duration, progress func(*daemon.ApiContextTracks)) (*daemon.ApiContextTracks, error) {
	deadline := time.Now().Add(timeout)
	for {
		res, err := e.ContextTracks(ctx, uri)
		if err != nil {
			return nil, err
		}
		if res.Ready && res.Cached >= res.Length {
			return res, nil
		}
		if progress != nil {
			progress(res)
		}
		if time.Now().After(deadline) {
			return res, nil
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
}

// Queue returns the previous, current and upcoming tracks.
func (e *Engine) Queue(ctx context.Context) (*daemon.ApiQueue, error) {
	return decode[daemon.ApiQueue](e.request(ctx, daemon.ApiRequestTypeGetQueue, nil))
}

// PlayContext plays a context, optionally starting at a given track.
func (e *Engine) PlayContext(ctx context.Context, contextURI, skipToURI string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlay, daemon.ApiPlay{Uri: contextURI, SkipToUri: skipToURI})
	return err
}

// AddToQueue appends a track to the user queue.
func (e *Engine) AddToQueue(ctx context.Context, uri string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeAddToQueue, uri)
	return err
}

// Liked reports which of the given track URIs are in Liked Songs.
func (e *Engine) Liked(ctx context.Context, uris []string) (map[string]bool, error) {
	res, err := decode[daemon.ApiLikedStates](e.request(ctx, daemon.ApiRequestTypeGetLiked, uris))
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(res.Items))
	for _, it := range res.Items {
		out[it.Uri] = it.Liked
	}
	return out, nil
}

// SetLiked adds tracks to or removes them from Liked Songs.
func (e *Engine) SetLiked(ctx context.Context, uris []string, liked bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeSetLiked, daemon.ApiSetLiked{Uris: uris, Liked: liked})
	return err
}

// Devices lists the account's Spotify Connect devices.
func (e *Engine) Devices(ctx context.Context) (*daemon.ApiConnectDevices, error) {
	return decode[daemon.ApiConnectDevices](e.request(ctx, daemon.ApiRequestTypeConnectDevices, nil))
}

// Transfer moves playback to a device; this device's own id pulls it here.
func (e *Engine) Transfer(ctx context.Context, deviceID string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeConnectTransfer, daemon.ApiConnectTransfer{DeviceId: deviceID})
	return err
}

// DjContextURI is the context Spotify's DJ plays from.
const DjContextURI = daemon.DjContextUri

// StartDj starts a fresh DJ session; while DJ already plays this moves it on
// to a new set.
func (e *Engine) StartDj(ctx context.Context) error {
	return e.PlayContext(ctx, DjContextURI, "")
}

// DjSuggestions returns prompts Spotify suggests asking DJ for now.
func (e *Engine) DjSuggestions(ctx context.Context) ([]string, error) {
	res, err := decode[daemon.ApiDjSuggestions](e.request(ctx, daemon.ApiRequestTypeDjSuggestions, nil))
	if err != nil {
		return nil, err
	}
	return res.Prompts, nil
}

// DjAsk asks DJ for something; DJ answers by playing what it put together.
func (e *Engine) DjAsk(ctx context.Context, prompt string, suggested bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeDjAsk, daemon.ApiDjAsk{Prompt: prompt, Suggested: suggested})
	return err
}

// AddToPlaylist appends tracks to one of the user's playlists.
func (e *Engine) AddToPlaylist(ctx context.Context, playlistURI string, uris []string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistAddTracks, daemon.ApiPlaylistAddTracks{PlaylistUri: playlistURI, Uris: uris})
	return err
}
