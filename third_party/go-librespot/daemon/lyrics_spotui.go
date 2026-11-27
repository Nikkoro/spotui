package daemon

// spotui patch, see third_party/PATCHES.md: a track's lyrics from Spotify,
// fetched off the player loop.

import (
	"context"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/spclient"
)

const ApiRequestTypeLyrics ApiRequestType = "lyrics"

// ApiLyrics asks for the lyrics of the track Uri.
type ApiLyrics struct {
	Uri     string
	Options spclient.LyricsOptions
}

const lyricsTimeout = 20 * time.Second

func (p *AppPlayer) handleLyricsRequest(req ApiRequest) (any, error) {
	data := req.Data.(ApiLyrics)
	id, err := librespot.SpotifyIdFromUri(data.Uri)
	if err != nil {
		return nil, err
	}
	spc := p.sess.Spclient()
	reply := apiReply(req)
	p.goDetached(lyricsTimeout, func(ctx context.Context) {
		l, err := spc.Lyrics(ctx, *id, data.Options)
		reply.done(l, err)
	})
	return nil, errReplyDeferred
}
