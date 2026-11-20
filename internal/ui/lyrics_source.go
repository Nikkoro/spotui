package ui

import (
	"context"
	"errors"
	"time"

	"github.com/devgianlu/go-librespot/spclient"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/lyrics"
)

// newLyricsClient asks Spotify first (when the player is there) and LRCLIB
// after it.
func newLyricsClient(cfg *config.Config, eng *engine.Engine) *lyrics.Client {
	c := lyrics.NewClient(cfg.CacheDir+"/lyrics", Version)
	if eng == nil {
		return c
	}
	return c.WithSpotify(func(ctx context.Context, uri string) (*lyrics.Lyrics, error) {
		l, err := eng.Lyrics(ctx, uri)
		var rl *spclient.LyricsRateLimitedError
		switch {
		case errors.Is(err, spclient.ErrNoLyrics):
			return nil, lyrics.ErrNotFound
		case errors.As(err, &rl):
			return nil, &lyrics.RateLimitedError{Until: rl.Until}
		case err != nil:
			return nil, err
		}
		return fromSpotifyLyrics(l), nil
	})
}

func fromSpotifyLyrics(l *spclient.Lyrics) *lyrics.Lyrics {
	out := &lyrics.Lyrics{Source: "Spotify"}
	if l.Provider != "" {
		out.Source += " · " + l.Provider
	}
	for _, line := range l.Lines {
		if l.Synced {
			out.Synced = append(out.Synced, lyrics.Line{At: time.Duration(line.StartMs) * time.Millisecond, Text: line.Words})
		} else {
			out.Plain = append(out.Plain, line.Words)
		}
	}
	return out
}
