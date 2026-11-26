package spclient

// spotui patch, see third_party/PATCHES.md: creating, renaming, deleting
// playlists and moving or inserting their items, through the same playlist
// service the Spotify apps use (no Web API rate limits).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"google.golang.org/protobuf/proto"
)

// PlaylistCreate makes a new, empty playlist owned by username and returns
// its URI. The service ignores the attributes of the request, so the name
// and description are set by a change after it (PlaylistSetAttributes), and
// it is not in the user's library yet: RootlistAdd puts it there.
func (c *Spclient) PlaylistCreate(ctx context.Context, username string) (string, error) {
	body, err := proto.Marshal(&playlist4pb.ListUpdateRequest{
		Info: &playlist4pb.ChangeInfo{
			User:      proto.String(username),
			Timestamp: proto.Int64(time.Now().UnixMilli()),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed marshalling ListUpdateRequest: %w", err)
	}

	// Not idempotent: resent after a lost answer it would make two.
	resp, err := c.RequestOnce(ctx, "POST", "/playlist/v2/playlist", nil, nil, body)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", &StatusError{Op: "playlist create", StatusCode: resp.StatusCode}
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading response body: %w", err)
	}
	var reply playlist4pb.CreateListReply
	if err := proto.Unmarshal(respBytes, &reply); err != nil {
		return "", fmt.Errorf("failed unmarshalling CreateListReply: %w", err)
	}
	if reply.GetUri() == "" {
		return "", fmt.Errorf("playlist create answered no uri")
	}
	return reply.GetUri(), nil
}

// PlaylistSetAttributes changes a playlist's name and description, as a
// change based on revision. A nil value is left as it is; an empty
// description removes it.
func (c *Spclient) PlaylistSetAttributes(ctx context.Context, playlist librespot.SpotifyId, username string, revision []byte, name, description *string) error {
	values := &playlist4pb.ListAttributes{}
	var none []playlist4pb.ListAttributeKind
	if name != nil {
		values.Name = proto.String(*name)
	}
	if description != nil {
		if *description == "" {
			none = append(none, playlist4pb.ListAttributeKind_LIST_DESCRIPTION)
		} else {
			values.Description = proto.String(*description)
		}
	}
	return c.playlistChange(ctx, playlist, username, revision, &playlist4pb.Op{
		Kind: playlist4pb.Op_UPDATE_LIST_ATTRIBUTES.Enum(),
		UpdateListAttributes: &playlist4pb.UpdateListAttributes{
			NewAttributes: &playlist4pb.ListAttributesPartialState{Values: values, NoValue: none},
		},
	})
}

// PlaylistMove moves the one item at from, which must be uri, so that it
// lands before the item now at to (len for the end), as a change based on
// revision.
func (c *Spclient) PlaylistMove(ctx context.Context, playlist librespot.SpotifyId, username string, revision []byte, uri string, from, to int) error {
	return c.playlistChange(ctx, playlist, username, revision, &playlist4pb.Op{
		Kind: playlist4pb.Op_MOV.Enum(),
		Mov: &playlist4pb.Mov{
			FromIndex: proto.Int32(int32(from)),
			Length:    proto.Int32(1),
			ToIndex:   proto.Int32(int32(to)),
			Items:     []*playlist4pb.Item{{Uri: proto.String(uri)}},
		},
	})
}

// PlaylistInsert puts uris into a playlist at position, as a change based
// on revision.
func (c *Spclient) PlaylistInsert(ctx context.Context, playlist librespot.SpotifyId, username string, revision []byte, uris []string, position int) error {
	items := make([]*playlist4pb.Item, 0, len(uris))
	for _, uri := range uris {
		items = append(items, &playlist4pb.Item{Uri: proto.String(uri)})
	}
	return c.playlistChange(ctx, playlist, username, revision, &playlist4pb.Op{
		Kind: playlist4pb.Op_ADD.Enum(),
		Add:  &playlist4pb.Add{FromIndex: proto.Int32(int32(position)), Items: items},
	})
}

// RootlistContents returns the user's rootlist revision and its entries'
// URIs in order (folder markers included), so an entry can be found by
// position.
func (c *Spclient) RootlistContents(ctx context.Context, username string) ([]byte, []string, error) {
	var revision []byte
	var uris []string
	for page := 0; page < playlistMaxPages; page++ {
		query := url.Values{}
		query.Set("decorate", "revision")
		query.Set("from", strconv.Itoa(len(uris)))
		query.Set("length", strconv.Itoa(playlistContentsPageSize))

		content, err := c.getList(ctx, rootlistPath(username), query)
		if err != nil {
			return nil, nil, err
		}
		if page == 0 {
			revision = content.GetRevision()
		} else if !bytes.Equal(revision, content.GetRevision()) {
			return nil, nil, ErrPlaylistConflict
		}

		items := content.GetContents().GetItems()
		for _, item := range items {
			uris = append(uris, item.GetUri())
		}
		if !content.GetContents().GetTruncated() || len(items) == 0 {
			return revision, uris, nil
		}
	}
	return nil, nil, fmt.Errorf("rootlist still truncated after %d pages", playlistMaxPages)
}

// RootlistAdd puts a playlist at the top of the user's library, public
// (shown on their profile) or not, as a change based on revision.
func (c *Spclient) RootlistAdd(ctx context.Context, username string, revision []byte, uri string, public bool) error {
	item := &playlist4pb.Item{
		Uri: proto.String(uri),
		Attributes: &playlist4pb.ItemAttributes{
			Timestamp: proto.Int64(time.Now().UnixMilli()),
			Public:    proto.Bool(public),
		},
	}
	return c.rootlistChange(ctx, username, revision, &playlist4pb.Op{
		Kind: playlist4pb.Op_ADD.Enum(),
		Add:  &playlist4pb.Add{FromIndex: proto.Int32(0), Items: []*playlist4pb.Item{item}},
	})
}

// RootlistRemove takes the entry at position, which must be uri, out of
// the user's library: the playlist is unfollowed (for one of their own,
// that is how the apps delete it).
func (c *Spclient) RootlistRemove(ctx context.Context, username string, revision []byte, uri string, position int) error {
	return c.rootlistChange(ctx, username, revision, &playlist4pb.Op{
		Kind: playlist4pb.Op_REM.Enum(),
		Rem: &playlist4pb.Rem{
			FromIndex: proto.Int32(int32(position)),
			Length:    proto.Int32(1),
			Items:     []*playlist4pb.Item{{Uri: proto.String(uri)}},
		},
	})
}

func rootlistPath(username string) string {
	return fmt.Sprintf("/playlist/v2/user/%s/rootlist", url.PathEscape(username))
}

func (c *Spclient) rootlistChange(ctx context.Context, username string, revision []byte, op *playlist4pb.Op) error {
	return c.listChange(ctx, rootlistPath(username), username, revision, op)
}

// listChange sends one operation as a change to the list at path.
func (c *Spclient) listChange(ctx context.Context, path, username string, revision []byte, op *playlist4pb.Op) error {
	body, err := proto.Marshal(&playlist4pb.ListChanges{
		BaseRevision: revision,
		Deltas: []*playlist4pb.Delta{{
			Ops: []*playlist4pb.Op{op},
			Info: &playlist4pb.ChangeInfo{
				User:      proto.String(username),
				Timestamp: proto.Int64(time.Now().UnixMilli()),
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("failed marshalling ListChanges: %w", err)
	}

	resp, err := c.RequestOnce(ctx, "POST", path+"/changes", nil, nil, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusConflict:
		return ErrPlaylistConflict
	default:
		return &StatusError{Op: "list changes", StatusCode: resp.StatusCode}
	}
}

func (c *Spclient) getList(ctx context.Context, path string, query url.Values) (*playlist4pb.SelectedListContent, error) {
	resp, err := c.Request(ctx, "GET", path, query, nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Op: "list", StatusCode: resp.StatusCode}
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %w", err)
	}
	var content playlist4pb.SelectedListContent
	if err := proto.Unmarshal(respBytes, &content); err != nil {
		return nil, fmt.Errorf("failed unmarshalling SelectedListContent: %w", err)
	}
	return &content, nil
}
