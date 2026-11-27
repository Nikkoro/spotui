package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/devgianlu/go-librespot/daemon"
	"github.com/devgianlu/go-librespot/spclient"
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

// SkipTo jumps to a track further on in the queue or the context.
func (e *Engine) SkipTo(ctx context.Context, uri string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeNext, daemon.ApiNext{Uri: &uri})
	return err
}

// Lyrics returns a track's lyrics from Spotify.
func (e *Engine) Lyrics(ctx context.Context, uri string, opt spclient.LyricsOptions) (*spclient.Lyrics, error) {
	return decode[spclient.Lyrics](e.request(ctx, daemon.ApiRequestTypeLyrics, daemon.ApiLyrics{Uri: uri, Options: opt}))
}

// CreatePlaylist makes a playlist, puts it at the top of the library and
// returns its URI. public shows it on the user's profile.
func (e *Engine) CreatePlaylist(ctx context.Context, name, description string, public bool) (string, error) {
	res, err := decode[daemon.ApiPlaylistCreated](e.request(ctx, daemon.ApiRequestTypePlaylistCreate,
		daemon.ApiPlaylistCreate{Name: name, Description: description, Public: public}))
	if err != nil {
		return "", err
	}
	return res.Uri, nil
}

// EditPlaylist renames a playlist or changes its description; nil leaves
// one as it is, an empty description removes it.
func (e *Engine) EditPlaylist(ctx context.Context, playlistURI string, name, description *string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistEdit,
		daemon.ApiPlaylistEdit{PlaylistUri: playlistURI, Name: name, Description: description})
	return err
}

// DeletePlaylist takes a playlist out of the library: unfollows it, which
// for the user's own is how the Spotify apps delete it.
func (e *Engine) DeletePlaylist(ctx context.Context, playlistURI string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistDelete, daemon.ApiPlaylistDelete{PlaylistUri: playlistURI})
	return err
}

// RemoveFromPlaylist removes the item at position, which must be uri.
func (e *Engine) RemoveFromPlaylist(ctx context.Context, playlistURI, uri string, position int) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistRemoveTrack,
		daemon.ApiPlaylistRemoveTrack{PlaylistUri: playlistURI, Uri: uri, Position: &position})
	return err
}

// InsertIntoPlaylist puts uris into a playlist at position.
func (e *Engine) InsertIntoPlaylist(ctx context.Context, playlistURI string, uris []string, position int) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistInsert,
		daemon.ApiPlaylistInsert{PlaylistUri: playlistURI, Uris: uris, Position: position})
	return err
}

// MovePlaylistItem moves the item at from, which must be uri, to index to.
func (e *Engine) MovePlaylistItem(ctx context.Context, playlistURI, uri string, from, to int) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlaylistMove,
		daemon.ApiPlaylistMove{PlaylistUri: playlistURI, Uri: uri, From: from, To: to})
	return err
}

// EpisodeState is how far an episode was listened to.
type EpisodeState = spclient.EpisodeState

// EpisodeStates returns the progress of the episodes among uris that were
// started, by URI.
func (e *Engine) EpisodeStates(ctx context.Context, uris []string) (map[string]EpisodeState, error) {
	res, err := decode[daemon.ApiEpisodeStatesReply](e.request(ctx, daemon.ApiRequestTypeEpisodeStates, daemon.ApiEpisodeStates{Uris: uris}))
	if err != nil {
		return nil, err
	}
	out := make(map[string]EpisodeState, len(res.Items))
	for _, s := range res.Items {
		out[s.Uri] = s
	}
	return out, nil
}

// SetEpisodePlayed marks an episode as played, or takes the mark off.
func (e *Engine) SetEpisodePlayed(ctx context.Context, uri string, played bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeEpisodePlayed, daemon.ApiEpisodePlayed{Uri: uri, Played: played})
	return err
}

// Radio returns the station of a track, artist, album or playlist; recent
// are a few of its tracks, for seeds without a station of their own.
func (e *Engine) Radio(ctx context.Context, seed string, recent []string) (string, error) {
	res, err := decode[daemon.ApiRadioStation](e.request(ctx, daemon.ApiRequestTypeRadio, daemon.ApiRadio{Seed: seed, RecentTracks: recent}))
	if err != nil {
		return "", err
	}
	return res.Uri, nil
}

// PlayRadio plays a station, starting with first (a song's radio starts
// with the song, put in front when the station does not hold it).
func (e *Engine) PlayRadio(ctx context.Context, station, first string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlay, daemon.ApiPlay{Uri: station, FirstUri: first})
	return err
}

// EditQueue changes the user's queue; see daemon.ApiQueueEdit for the
// edits.
func (e *Engine) EditQueue(ctx context.Context, edit daemon.ApiQueueEdit) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeQueueEdit, edit)
	return err
}
