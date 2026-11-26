package daemon

// spotui patch, see third_party/PATCHES.md: episode progress for a list of
// episodes, and marking one as played, off the player loop.

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/spclient"
)

const (
	ApiRequestTypeEpisodeStates ApiRequestType = "episode_states"
	ApiRequestTypeEpisodePlayed ApiRequestType = "episode_played"
)

// ApiEpisodeStates asks for the progress of the episodes Uris.
type ApiEpisodeStates struct {
	Uris []string
}

// ApiEpisodeStatesReply answers with the episodes that were started.
type ApiEpisodeStatesReply struct {
	Items []spclient.EpisodeState
}

// ApiEpisodePlayed marks Uri as played, or takes the mark off.
type ApiEpisodePlayed struct {
	Uri    string
	Played bool
}

func (p *AppPlayer) handlePodcastRequest(req ApiRequest) (any, error) {
	spc := p.sess.Spclient()
	reply := apiReply(req)
	switch data := req.Data.(type) {
	case ApiEpisodeStates:
		p.goDetached(resumeTimeout, func(ctx context.Context) {
			states, err := spc.EpisodeStates(ctx, data.Uris)
			if err != nil {
				reply.done(nil, err)
				return
			}
			reply.done(&ApiEpisodeStatesReply{Items: states}, nil)
		})
	case ApiEpisodePlayed:
		id, err := librespot.SpotifyIdFromUri(data.Uri)
		if err != nil || id.Type() != librespot.SpotifyIdTypeEpisode {
			return nil, ErrBadRequest
		}
		p.goDetached(resumeTimeout, func(ctx context.Context) {
			reply.done(nil, spc.SetEpisodePlayed(ctx, *id, data.Played))
		})
	default:
		return nil, errors.New("bad podcast request")
	}
	return nil, errReplyDeferred
}

// collectionSetYourEpisodes holds the episodes saved to Your Episodes,
// where the Spotify apps' "save episode" puts them.
const collectionSetYourEpisodes = "listenlater"

// collectionSetOf is the collection set "like" means for uri: Liked Songs,
// or Your Episodes for an episode.
func collectionSetOf(uri string) string {
	if id, err := librespot.SpotifyIdFromUri(uri); err == nil && id.Type() == librespot.SpotifyIdTypeEpisode {
		return collectionSetYourEpisodes
	}
	return spclient.CollectionSetLikedSongs
}

// collectionWriteBySet saves or removes uris, each in its own set.
func collectionWriteBySet(ctx context.Context, spc *spclient.Spclient, username string, uris []string, remove bool) error {
	bySet := map[string][]string{}
	for _, uri := range uris {
		set := collectionSetOf(uri)
		bySet[set] = append(bySet[set], uri)
	}
	for set, batch := range bySet {
		if err := spc.CollectionWrite(ctx, username, set, batch, remove); err != nil {
			return err
		}
	}
	return nil
}

// collectionContainsBySet tells for each of uris whether it is saved, each
// asked of its own set, in the order of uris.
func collectionContainsBySet(ctx context.Context, spc *spclient.Spclient, username string, uris []string) ([]bool, error) {
	out := make([]bool, len(uris))
	idx := map[string][]int{}
	for i, uri := range uris {
		set := collectionSetOf(uri)
		idx[set] = append(idx[set], i)
	}
	for set, positions := range idx {
		batch := make([]string, len(positions))
		for j, i := range positions {
			batch[j] = uris[i]
		}
		found, err := spc.CollectionContains(ctx, username, set, batch)
		if err != nil {
			return nil, err
		}
		for j, i := range positions {
			out[i] = found[j]
		}
	}
	return out, nil
}

// yourEpisodesContextUri is the context the Spotify apps play Your
// Episodes as.
func yourEpisodesContextUri(username string) string {
	return "spotify:user:" + username + ":collection:your-episodes"
}

// resumeReportEvery is how often the progress of a playing episode is
// saved, so quitting (or a crash) loses at most this much.
const resumeReportEvery = 30 * time.Second

// reportResumeWhilePlaying saves the playing episode's position; the
// player loop calls it every resumeReportEvery.
func (p *AppPlayer) reportResumeWhilePlaying() {
	if p.primaryStream == nil || p.state.player.IsPaused {
		return
	}
	p.reportResumePosition(p.primaryStream, p.player.PositionMs())
}

// reportResumeOnExit saves the episode's position as the player stops:
// upstream saves it on pause and on the next track only, so quitting while
// an episode played lost the place. The player's context is already
// cancelled here, so this waits on its own short one.
func (p *AppPlayer) reportResumeOnExit() {
	stream := p.primaryStream
	if stream == nil || stream.RequestedId.Type() != librespot.SpotifyIdTypeEpisode ||
		bytes.Equal(stream.PlaybackId, p.resumeFinishedPlaybackId) {
		return
	}
	pos := p.player.PositionMs()
	if pos <= 0 {
		return
	}
	// Short: spotui gives the player 3 s to stop.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.sess.Spclient().SetResumePositionMs(ctx, stream.RequestedId, pos); err != nil {
		p.app.log.WithError(err).Warn("failed saving the episode's place on exit")
		return
	}
	p.app.log.WithField("uri", stream.RequestedId.Uri()).Debugf("saved episode position %dms on exit", pos)
}

// unavailableTTL is how long a uri without metadata is taken as gone
// before it is asked for again.
const unavailableTTL = 30 * time.Minute

// unavailableSet holds the uris Spotify answered without metadata.
type unavailableSet struct {
	mu   sync.Mutex
	uris map[string]time.Time
}

func (s *unavailableSet) add(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uris == nil {
		s.uris = map[string]time.Time{}
	}
	s.uris[uri] = time.Now()
}

func (s *unavailableSet) has(uri string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.uris[uri]
	if ok && time.Since(at) > unavailableTTL {
		delete(s.uris, uri)
		return false
	}
	return ok
}
