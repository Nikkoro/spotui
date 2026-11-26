package spclient

// spotui patch, see third_party/PATCHES.md: the progress of many episodes in
// one request, and marking an episode as played or not.

import (
	"context"
	"fmt"
	"strings"

	librespot "github.com/devgianlu/go-librespot"
	resumptionpb "github.com/devgianlu/go-librespot/proto/spotify/resumption/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// EpisodeState is how far an episode was listened to.
type EpisodeState struct {
	Uri        string
	PositionMs int64
	Finished   bool
}

// maxStatesPerRequest caps the uris of one ListCurrentStates filter.
const maxStatesPerRequest = 100

// EpisodeStates reports the progress of the episodes it knows of among uris;
// episodes never started are left out.
func (c *Spclient) EpisodeStates(ctx context.Context, uris []string) ([]EpisodeState, error) {
	var out []EpisodeState
	for start := 0; start < len(uris); start += maxStatesPerRequest {
		batch := uris[start:min(start+maxStatesPerRequest, len(uris))]
		quoted := make([]string, 0, len(batch))
		for _, uri := range batch {
			id, err := librespot.SpotifyIdFromUri(uri)
			if err != nil {
				return nil, err
			}
			// A SpotifyId renders as spotify:<type>:<base62>, so it cannot
			// escape the CEL string literal.
			quoted = append(quoted, fmt.Sprintf("%q", id.Uri()))
		}
		resp, err := c.ListCurrentStates(ctx, &resumptionpb.ListCurrentStatesRequest{
			PageSize: int32(len(batch)),
			Filter:   "cs.uri in [" + strings.Join(quoted, ", ") + "]",
		})
		if err != nil {
			return nil, err
		}
		for _, state := range resp.CurrentStates {
			if s, ok := episodeState(state); ok {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// episodeState reads the most recent resume point of a state.
func episodeState(state *resumptionpb.CurrentState) (EpisodeState, bool) {
	var latest *resumptionpb.ResumePointRevision
	for _, rev := range state.ResumePointRevisions {
		if latest == nil || rev.UpdateTime.AsTime().After(latest.UpdateTime.AsTime()) {
			latest = rev
		}
	}
	if latest == nil {
		return EpisodeState{}, false
	}
	s := EpisodeState{Uri: state.Uri}
	switch point := latest.Snapshot.GetResumePoint().(type) {
	case *resumptionpb.ResumePoint_Position:
		s.PositionMs = point.Position.AsDuration().Milliseconds()
	case *resumptionpb.ResumePoint_RelativePosition:
		s.PositionMs = point.RelativePosition.StartOffset.AsDuration().Milliseconds()
	case *resumptionpb.ResumePoint_Finished, *resumptionpb.ResumePoint_MarkedAsFinished:
		s.Finished = true
	case *resumptionpb.ResumePoint_UnmarkedAsFinished:
	default:
		return EpisodeState{}, false
	}
	return s, true
}

// SetEpisodePlayed marks an episode as played, or takes the mark off (it
// then starts from the beginning), as the Spotify apps' "Mark as played".
func (c *Spclient) SetEpisodePlayed(ctx context.Context, id librespot.SpotifyId, played bool) error {
	point := &resumptionpb.ResumePoint{ResumePoint: &resumptionpb.ResumePoint_UnmarkedAsFinished{UnmarkedAsFinished: &emptypb.Empty{}}}
	if played {
		point = &resumptionpb.ResumePoint{ResumePoint: &resumptionpb.ResumePoint_MarkedAsFinished{MarkedAsFinished: &emptypb.Empty{}}}
	}
	return c.createResumePoint(ctx, id, point)
}
