package daemon

// spotui patch, see third_party/PATCHES.md: playlist management (create,
// rename, delete, move and insert items), off the player loop.

import (
	"context"
	"errors"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/spclient"
)

const (
	ApiRequestTypePlaylistCreate ApiRequestType = "playlist_create"
	ApiRequestTypePlaylistEdit   ApiRequestType = "playlist_edit"
	ApiRequestTypePlaylistDelete ApiRequestType = "playlist_delete"
	ApiRequestTypePlaylistMove   ApiRequestType = "playlist_move"
	ApiRequestTypePlaylistInsert ApiRequestType = "playlist_insert"
)

// ApiPlaylistCreate makes a playlist and puts it at the top of the library.
type ApiPlaylistCreate struct {
	Name, Description string
	Public            bool
}

// ApiPlaylistCreated answers ApiPlaylistCreate.
type ApiPlaylistCreated struct {
	Uri string
}

// ApiPlaylistEdit changes a playlist's name or description; nil leaves one
// as it is, an empty description removes it.
type ApiPlaylistEdit struct {
	PlaylistUri       string
	Name, Description *string
}

// ApiPlaylistDelete takes a playlist out of the library (unfollows it).
type ApiPlaylistDelete struct {
	PlaylistUri string
}

// ApiPlaylistMove moves the item at From, which must be Uri, so that it
// ends up at index To.
type ApiPlaylistMove struct {
	PlaylistUri, Uri string
	From, To         int
}

// ApiPlaylistInsert puts Uris into a playlist at Position.
type ApiPlaylistInsert struct {
	PlaylistUri string
	Uris        []string
	Position    int
}

func (p *AppPlayer) handlePlaylistRequest(req ApiRequest) (any, error) {
	spc, username := p.sess.Spclient(), p.sess.Username()
	reply := apiReply(req)
	run := func(what, invalidate string, fn func(ctx context.Context) (any, error)) {
		p.goDetached(libraryRequestTimeout, func(ctx context.Context) {
			res, err := fn(ctx)
			if err != nil {
				reply.done(nil, libraryError(what, err))
				return
			}
			if invalidate != "" {
				p.app.contextLists.invalidate(invalidate)
			}
			reply.done(res, nil)
		})
	}

	switch data := req.Data.(type) {
	case ApiPlaylistCreate:
		run("failed creating playlist", "", func(ctx context.Context) (any, error) {
			uri, err := createPlaylist(ctx, spc, username, data)
			if err != nil {
				return nil, err
			}
			return &ApiPlaylistCreated{Uri: uri}, nil
		})
	case ApiPlaylistEdit:
		run("failed editing playlist", "", func(ctx context.Context) (any, error) {
			return nil, retryOnConflict(ctx, spc, data.PlaylistUri, func(id librespot.SpotifyId, revision []byte, _ []string) error {
				return spc.PlaylistSetAttributes(ctx, id, username, revision, data.Name, data.Description)
			})
		})
	case ApiPlaylistDelete:
		run("failed deleting playlist", data.PlaylistUri, func(ctx context.Context) (any, error) {
			return nil, deletePlaylist(ctx, spc, username, data.PlaylistUri)
		})
	case ApiPlaylistMove:
		run("failed moving in playlist", data.PlaylistUri, func(ctx context.Context) (any, error) {
			return nil, retryOnConflict(ctx, spc, data.PlaylistUri, func(id librespot.SpotifyId, revision []byte, uris []string) error {
				from := data.From
				if _, err := itemPosition(uris, data.Uri, &from); err != nil {
					return err
				}
				if data.To < 0 || data.To >= len(uris) {
					return errors.New("move target out of range")
				}
				return spc.PlaylistMove(ctx, id, username, revision, data.Uri, from, insertBefore(from, data.To))
			})
		})
	case ApiPlaylistInsert:
		run("failed adding to playlist", data.PlaylistUri, func(ctx context.Context) (any, error) {
			return nil, retryOnConflict(ctx, spc, data.PlaylistUri, func(id librespot.SpotifyId, revision []byte, uris []string) error {
				pos := min(max(data.Position, 0), len(uris))
				return spc.PlaylistInsert(ctx, id, username, revision, data.Uris, pos)
			})
		})
	default:
		return nil, errors.New("bad playlist request")
	}
	return nil, errReplyDeferred
}

// retryOnConflict reads the playlist (revision and items) and applies fn,
// once more when the playlist moved on in between.
func retryOnConflict(ctx context.Context, spc *spclient.Spclient, playlistUri string, fn func(id librespot.SpotifyId, revision []byte, uris []string) error) error {
	id, err := librespot.SpotifyIdFromUri(playlistUri)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		revision, uris, err := spc.PlaylistContents(ctx, *id)
		if err == nil {
			err = fn(*id, revision, uris)
		}
		if !errors.Is(err, spclient.ErrPlaylistConflict) || errors.Is(err, errPlaylistItemMoved) || attempt > 0 {
			return err
		}
	}
}

// createPlaylist makes the playlist, names it, then adds it to the
// library.
func createPlaylist(ctx context.Context, spc *spclient.Spclient, username string, data ApiPlaylistCreate) (string, error) {
	uri, err := spc.PlaylistCreate(ctx, username)
	if err != nil {
		return "", err
	}
	var desc *string
	if data.Description != "" {
		desc = &data.Description
	}
	if err := retryOnConflict(ctx, spc, uri, func(id librespot.SpotifyId, revision []byte, _ []string) error {
		return spc.PlaylistSetAttributes(ctx, id, username, revision, &data.Name, desc)
	}); err != nil {
		return uri, err
	}
	for attempt := 0; ; attempt++ {
		revision, _, err := spc.RootlistContents(ctx, username)
		if err == nil {
			err = spc.RootlistAdd(ctx, username, revision, uri, data.Public)
		}
		if !errors.Is(err, spclient.ErrPlaylistConflict) || attempt > 0 {
			return uri, err
		}
	}
}

// deletePlaylist takes the first entry of playlistUri out of the library.
func deletePlaylist(ctx context.Context, spc *spclient.Spclient, username, playlistUri string) error {
	for attempt := 0; ; attempt++ {
		revision, uris, err := spc.RootlistContents(ctx, username)
		if err != nil {
			return err
		}
		pos, err := itemPosition(uris, playlistUri, nil)
		if err != nil {
			return err
		}
		err = spc.RootlistRemove(ctx, username, revision, playlistUri, pos)
		if !errors.Is(err, spclient.ErrPlaylistConflict) || attempt > 0 {
			return err
		}
	}
}

// insertBefore turns the index an item should end up at into the index,
// counted before the move, of the item it goes in front of: what the
// playlist service's move takes (as the Web API's insert_before).
func insertBefore(from, to int) int {
	if to > from {
		return to + 1
	}
	return to
}
