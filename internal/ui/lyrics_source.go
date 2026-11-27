package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	probe := lyricsProbeFromEnv()
	return c.WithSpotify(func(ctx context.Context, uri string) (*lyrics.Lyrics, error) {
		l, err := eng.Lyrics(ctx, uri, probe.opt)
		if err == nil {
			probe.report(eng, uri, l.Raw)
		}
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

// lyricsProbe is for finding out how Spotify sends lyric translations
// (issue #63): SPOTUI_LYRICS_LANG sets Accept-Language, SPOTUI_LYRICS_QUERY
// adds query parameters ("a=1&b=2"), SPOTUI_LYRICS_DUMP is a folder each
// response is written to as <track id>.json. With --debug, every response
// gets a line on its shape (keys, alternatives and their languages).
type lyricsProbe struct {
	opt  spclient.LyricsOptions
	dump string
}

func lyricsProbeFromEnv() lyricsProbe {
	p := lyricsProbe{dump: os.Getenv("SPOTUI_LYRICS_DUMP")}
	p.opt.Language = os.Getenv("SPOTUI_LYRICS_LANG")
	if q, err := url.ParseQuery(os.Getenv("SPOTUI_LYRICS_QUERY")); err == nil && len(q) > 0 {
		p.opt.Query = q
	}
	return p
}

func (p lyricsProbe) report(eng *engine.Engine, uri string, raw []byte) {
	if len(raw) == 0 {
		return
	}
	eng.Debugf("lyrics: color-lyrics for %s (Accept-Language %q, query %q): %s", uri, p.opt.Language, p.opt.Query.Encode(), lyricsShape(raw))
	if p.dump == "" {
		return
	}
	id := uri[strings.LastIndex(uri, ":")+1:]
	if err := os.MkdirAll(p.dump, 0o755); err == nil {
		err = os.WriteFile(filepath.Join(p.dump, id+".json"), raw, 0o644)
		if err != nil {
			eng.Debugf("lyrics: dump: %v", err)
		}
	}
}

// lyricsShape describes a color-lyrics response without its text: the
// keys at the top and in "lyrics", and for each of "alternatives" its
// language, how many lines and its keys.
func lyricsShape(raw []byte) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return "not JSON: " + err.Error()
	}
	var b strings.Builder
	b.WriteString("keys " + strings.Join(sortedKeys(top), ","))
	var ly map[string]json.RawMessage
	if json.Unmarshal(top["lyrics"], &ly) == nil {
		b.WriteString("; lyrics keys " + strings.Join(sortedKeys(ly), ","))
		var lang string
		_ = json.Unmarshal(ly["language"], &lang)
		b.WriteString("; language " + lang)
	}
	alts := top["alternatives"]
	if a, ok := ly["alternatives"]; ok && len(alts) == 0 {
		alts = a
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(alts, &list); err != nil || len(list) == 0 {
		b.WriteString("; no alternatives")
		return b.String()
	}
	b.WriteString("; alternatives " + strconv.Itoa(len(list)) + ":")
	for _, a := range list {
		var lang string
		_ = json.Unmarshal(a["language"], &lang)
		var lines []json.RawMessage
		_ = json.Unmarshal(a["lines"], &lines)
		b.WriteString(" [" + lang + " " + strconv.Itoa(len(lines)) + " lines, keys " + strings.Join(sortedKeys(a), ",") + "]")
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
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
