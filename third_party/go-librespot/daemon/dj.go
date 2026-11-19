package daemon

// spotui patch, see third_party/PATCHES.md: "Ask DJ" and DJ prompt
// suggestions through the API.

import (
	"context"
	"errors"
	"strings"
	"time"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
	"github.com/devgianlu/go-librespot/spclient"
	"github.com/devgianlu/go-librespot/tracks"
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

// "Ask DJ" answers. DJ takes a request by sending this device a play of its
// own context whose metadata acknowledges it (interactivity_response) and
// carries the line to say (narration.welcome.*). The tracks it picked arrive
// in the session moments later, marked with idj_request_id. Until they do
// the answer waits; then playback jumps to the first of them, and the welcome
// is spoken before it. Modelled on librespot-org/librespot#1797.

const (
	djInteractivityResponse = "interactivity_response"
	djRequestIdKey          = "idj_request_id"
	djAnswerWait            = 30 * time.Second
)

type djPending struct {
	welcome map[string]string
	at      time.Time
	// request the playing track answers, so its own segment is not mistaken
	// for the new one
	current string
}

func (p *AppPlayer) isDjAnswer(spotCtx *connectpb.Context) bool {
	return spotCtx != nil &&
		spotCtx.Metadata[djInteractivityResponse] != "" &&
		spotCtx.Uri == DjContextUri &&
		p.state.player.ContextUri == DjContextUri
}

func (p *AppPlayer) handleDjAnswer(spotCtx *connectpb.Context) error {
	welcome := map[string]string{}
	for k, v := range spotCtx.Metadata {
		if strings.HasPrefix(k, narrationWelcomePrefix+".") {
			welcome[k] = v
		}
	}
	p.djAnswer = &djPending{
		welcome: welcome,
		at:      time.Now(),
		current: p.state.player.Track.GetMetadata()[djRequestIdKey],
	}
	p.app.log.Debugf("DJ took the request, waiting for its segment")

	// The segment may already be in the session.
	p.playDjAnswer()
	return nil
}

func (a *djPending) requested(track *connectpb.ContextTrack) bool {
	id := track.GetMetadata()[djRequestIdKey]
	return id != "" && id != a.current
}

// playDjAnswer jumps to the answer's first track if the current list has it.
func (p *AppPlayer) playDjAnswer() {
	ans := p.djAnswer
	if ans == nil {
		return
	}
	if time.Since(ans.at) > djAnswerWait {
		p.app.log.Warnf("DJ's answer to a request never arrived")
		p.djAnswer = nil
		return
	}

	p.listJob("dj answer", classLoad, nil,
		func(ctx context.Context, list *tracks.List) error {
			// Seek, not TrySeek: when the segment is not here yet the list
			// must stay where it is.
			return list.Seek(ctx, ans.requested)
		},
		func(p *AppPlayer, snap *tracks.Snapshot, err error) {
			if err != nil || p.djAnswer != ans {
				return // not here yet; the context update brings it
			}
			p.djAnswer = nil
			p.djWelcome = ans.welcome

			p.publishSnapshot(snap)
			p.state.player.Timestamp = time.Now().UnixMilli()
			p.state.player.PositionAsOfTimestamp = 0
			p.loadCurrentTrack(p.state.player.IsPaused, true, true, 0, func(err error) {
				if err != nil {
					p.app.log.WithError(err).Warn("failed playing DJ's answer")
				}
			})
		})
}

// loadDjAnswerContext plays a context update that carries the segment a
// pending request is waiting for, and reports whether it did.
func (p *AppPlayer) loadDjAnswerContext(spotCtx *connectpb.Context) bool {
	ans := p.djAnswer
	if ans == nil || spotCtx == nil || spotCtx.Uri != DjContextUri {
		return false
	}
	if time.Since(ans.at) > djAnswerWait {
		p.djAnswer = nil
		return false
	}

	found := false
	for _, page := range spotCtx.Pages {
		for _, t := range page.Tracks {
			if ans.requested(t) {
				found = true
				break
			}
		}
	}
	if !found {
		// Metadata only: the tracks may still have reached the list.
		p.playDjAnswer()
		return false
	}

	p.djAnswer = nil
	p.djWelcome = ans.welcome
	// Keep DJ's session: later requests name it.
	sessionId := p.state.player.SessionId
	p.loadContext(spotCtx, ans.requested, p.state.player.IsPaused, true, func(err error) {
		if err != nil {
			p.app.log.WithError(err).Warn("failed loading DJ's answer")
		}
	})
	p.state.player.SessionId = sessionId
	return true
}
