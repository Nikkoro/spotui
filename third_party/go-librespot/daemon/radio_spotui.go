package daemon

// spotui patch, see third_party/PATCHES.md: the radio of a track, artist,
// album or playlist, as the Spotify apps' "Go to radio". A radio is a
// station context, which the player plays on without end, as autoplay
// does at the end of a playlist.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
	playerpb "github.com/devgianlu/go-librespot/proto/spotify/player"
	"github.com/devgianlu/go-librespot/spclient"
	"google.golang.org/protobuf/proto"
)

const ApiRequestTypeRadio ApiRequestType = "radio"

// ApiRadio asks for the station of Seed. RecentTracks are a few of its
// tracks, which the autoplay service bases a station on when Seed has no
// station of its own.
type ApiRadio struct {
	Seed         string
	RecentTracks []string
}

// ApiRadioStation answers ApiRadio.
type ApiRadioStation struct {
	Uri string
}

const radioTimeout = 15 * time.Second

func (p *AppPlayer) handleRadioRequest(req ApiRequest) (any, error) {
	data := req.Data.(ApiRadio)
	switch librespot.ContextUriType(data.Seed) {
	case "track", "artist", "album", "playlist":
	default:
		return nil, ErrBadRequest
	}
	spc := p.sess.Spclient()
	reply := apiReply(req)
	p.goDetached(radioTimeout, func(ctx context.Context) {
		// The radio playlist the Spotify apps play, when there is one; the
		// station chain below otherwise.
		if uri, err := spc.RadioPlaylist(ctx, data.Seed); err == nil {
			p.app.log.Debugf("radio of %s: %s", data.Seed, uri)
			reply.done(&ApiRadioStation{Uri: uri}, nil)
			return
		} else {
			p.app.log.WithError(err).Debugf("no radio playlist for %s, trying stations", data.Seed)
		}
		uri, err := radioStation(ctx, data, func(ctx context.Context, uri string) (bool, error) {
			spotCtx, err := spc.ContextResolve(ctx, uri)
			if err != nil {
				return false, err
			}
			// A station with tracks, or a page to fetch them from.
			for _, page := range spotCtx.GetPages() {
				if len(page.GetTracks()) > 0 || page.GetPageUrl() != "" || page.GetNextPageUrl() != "" {
					return true, nil
				}
			}
			return false, nil
		}, func(ctx context.Context, seed string, recent []string) (string, error) {
			spotCtx, err := spc.ContextResolveAutoplay(ctx, &playerpb.AutoplayContextRequest{
				ContextUri:     proto.String(seed),
				RecentTrackUri: recent,
			})
			if err != nil {
				return "", err
			}
			return spotCtx.GetUri(), nil
		}, func(ctx context.Context) []string {
			return firstContextTracks(ctx, p.app.log, spc, data.Seed, 5)
		})
		if err != nil {
			p.app.log.WithError(err).Warnf("no radio for %s", data.Seed)
			reply.done(nil, err)
			return
		}
		p.app.log.Debugf("radio of %s: %s", data.Seed, uri)
		reply.done(&ApiRadioStation{Uri: uri}, nil)
	})
	return nil, errReplyDeferred
}

// radioStation finds the station of a seed: its own station
// (spotify:station:<type>:<id>) when Spotify has one, otherwise the one the
// autoplay service makes of a few of its tracks, and last the station of
// one of those tracks (Spotify has one for nearly every song).
func radioStation(ctx context.Context, data ApiRadio,
	resolves func(ctx context.Context, uri string) (bool, error),
	autoplay func(ctx context.Context, seed string, recent []string) (string, error),
	seedTracks func(ctx context.Context) []string,
) (string, error) {
	station := "spotify:station:" + strings.TrimPrefix(data.Seed, "spotify:")
	ok, err := resolves(ctx, station)
	if ok {
		return station, nil
	}
	errs := []error{err}

	recent := data.RecentTracks
	switch {
	case len(recent) > 0:
	case strings.HasPrefix(data.Seed, "spotify:track:"):
		recent = []string{data.Seed}
	default:
		recent = seedTracks(ctx)
	}
	uri, err := autoplay(ctx, data.Seed, recent)
	if err == nil && strings.HasPrefix(uri, "spotify:station:") {
		return uri, nil
	}
	errs = append(errs, err)

	if !strings.HasPrefix(data.Seed, "spotify:track:") {
		for _, t := range recent[:min(len(recent), 2)] {
			station := "spotify:station:" + strings.TrimPrefix(t, "spotify:")
			if ok, err := resolves(ctx, station); ok {
				return station, nil
			} else {
				errs = append(errs, err)
			}
		}
	}
	return "", fmt.Errorf("no radio for %s: %w", data.Seed, errors.Join(errs...))
}

// prependContextTrack puts uri at the front of a context's first page,
// moving it there when a page the context carries holds it further on.
func prependContextTrack(spotCtx *connectpb.Context, uri string) {
	for _, page := range spotCtx.Pages {
		for i, t := range page.Tracks {
			if t.Uri == uri {
				page.Tracks = append(page.Tracks[:i], page.Tracks[i+1:]...)
				break
			}
		}
	}
	track := &connectpb.ContextTrack{Uri: uri, Metadata: map[string]string{}}
	if len(spotCtx.Pages) > 0 && spotCtx.Pages[0].PageUrl == "" {
		spotCtx.Pages[0].Tracks = append([]*connectpb.ContextTrack{track}, spotCtx.Pages[0].Tracks...)
		return
	}
	// The first page is still to be fetched: a page of its own in front.
	spotCtx.Pages = append([]*connectpb.ContextPage{{Tracks: []*connectpb.ContextTrack{track}}}, spotCtx.Pages...)
}

// firstContextTracks returns up to n track uris from the first pages of a
// context, or none when it cannot be read.
func firstContextTracks(ctx context.Context, log librespot.Logger, spc *spclient.Spclient, uri string, n int) []string {
	spotCtx, err := spc.ContextResolve(ctx, uri)
	if err != nil {
		return nil
	}
	resolver, err := spclient.NewContextResolver(ctx, log, spc, spotCtx)
	if err != nil {
		return nil
	}
	var out []string
	for page := 0; page < 3 && len(out) < n; page++ {
		tracks, err := resolver.Page(ctx, page)
		if err != nil {
			break
		}
		for _, t := range tracks {
			if strings.HasPrefix(t.GetUri(), "spotify:track:") && len(out) < n {
				out = append(out, t.GetUri())
			}
		}
	}
	return out
}
