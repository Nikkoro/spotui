package daemon

// spotui patch, see third_party/PATCHES.md: "Ask DJ" and DJ prompt
// suggestions through the API.

import (
	"context"
	"errors"
	"time"

	"github.com/devgianlu/go-librespot/spclient"
)

// DjContextUri is the context Spotify's DJ plays from.
const DjContextUri = "spotify:playlist:37i9dQZF1EYkqdzj48dyYq"

const (
	ApiRequestTypeDjSuggestions ApiRequestType = "dj_suggestions"
	ApiRequestTypeDjAsk         ApiRequestType = "dj_ask"
)

// ApiDjAsk is a request typed for DJ, or one of its suggestions.
type ApiDjAsk struct {
	Prompt    string
	Suggested bool
}

// ApiDjSuggestions are prompts to offer for the current track.
type ApiDjSuggestions struct {
	Prompts []string
}

const djTimeout = 30 * time.Second

func (p *AppPlayer) handleDjRequest(req ApiRequest) (any, error) {
	track := p.state.player.Track
	trackUri := track.GetUri()
	spc := p.sess.Spclient()
	reply := apiReply(req)

	switch req.Type {
	case ApiRequestTypeDjSuggestions:
		p.goDetached(djTimeout, func(ctx context.Context) {
			prompts, err := spc.DjSuggestions(ctx, trackUri)
			reply.done(&ApiDjSuggestions{Prompts: prompts}, err)
		})

	case ApiRequestTypeDjAsk:
		if p.state.player.ContextUri != DjContextUri {
			return nil, errors.New("DJ takes requests only while it plays")
		}
		data := req.Data.(ApiDjAsk)
		ask := &spclient.DjAskRequest{
			Prompt:           data.Prompt,
			SessionId:        p.state.player.SessionId,
			DeviceId:         p.app.deviceId,
			CurrentSegmentId: track.GetMetadata()["segment"],
			CurrentTrackUri:  trackUri,
			Input:            spclient.DjPromptText,
		}
		if data.Suggested {
			ask.Input = spclient.DjPromptSuggested
		}
		p.goDetached(djTimeout, func(ctx context.Context) {
			reply.done(nil, spc.DjAsk(ctx, ask))
		})
	}

	return nil, errReplyDeferred
}
