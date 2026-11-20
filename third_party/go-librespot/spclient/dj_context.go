package spclient

// spotui patch, see third_party/PATCHES.md: resolving DJ's context.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	connectpb "github.com/devgianlu/go-librespot/proto/spotify/connectstate"
)

// djContextUri is the context Spotify's DJ plays from (daemon.DjContextUri).
const djContextUri = "spotify:playlist:37i9dQZF1EYkqdzj48dyYq"

// lexiconContextUrl names, in DJ's context, the session that holds its tracks.
const lexiconContextUrl = "lexicon_context_url"

// djContext resolves DJ the way Spotify's clients do: context resolve answers
// with an empty shell whose metadata names a lexicon session; the session
// holds the sets played so far, and the next page of it is a fresh set.
func (c *Spclient) djContext(ctx context.Context, uri string) (*connectpb.Context, error) {
	shell, err := c.contextResolveRaw(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("DJ: %w", err)
	}
	if hasTracks(shell) {
		return shell, nil
	}
	session := shell.Metadata[lexiconContextUrl]
	if !isLexiconUrl(session) {
		return nil, fmt.Errorf("DJ is not available for this account (no session in its context)")
	}
	c.log.Debugf("resolving DJ through its session %s", session)

	spotCtx, err := c.ContextResolveUrl(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("DJ session: %w", err)
	}

	// The session holds every set since it began; starting DJ asks for a new
	// one, as "Let DJ pick" does. Without one, resume the session.
	if n := len(spotCtx.Pages); n > 0 && isLexiconUrl(spotCtx.Pages[n-1].NextPageUrl) {
		if page, err := c.lexiconPage(ctx, spotCtx.Pages[n-1].NextPageUrl); err == nil && len(page.Tracks) > 0 {
			spotCtx.Pages = []*connectpb.ContextPage{page}
		} else if err != nil {
			c.log.WithError(err).Warnf("no new DJ set, resuming its session")
		}
	}

	if spotCtx.Metadata == nil {
		spotCtx.Metadata = map[string]string{}
	}
	for k, v := range shell.Metadata {
		if _, ok := spotCtx.Metadata[k]; !ok {
			spotCtx.Metadata[k] = v
		}
	}
	spotCtx.Uri = uri
	spotCtx.Url = "context://" + uri
	if !hasTracks(spotCtx) {
		return nil, fmt.Errorf("DJ session has no tracks")
	}
	return spotCtx, nil
}

// lexiconPage loads one page of a DJ session; the answer is the page alone.
func (c *Spclient) lexiconPage(ctx context.Context, hmUrl string) (*connectpb.ContextPage, error) {
	resp, err := c.RequestHm(ctx, "GET", hmUrl, nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, &ContextResolveError{StatusCode: resp.StatusCode, url: hmUrl}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var page connectpb.ContextPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed json unmarshalling DJ page: %w", err)
	}
	return &page, nil
}

func hasTracks(spotCtx *connectpb.Context) bool {
	for _, page := range spotCtx.GetPages() {
		if len(page.Tracks) > 0 {
			return true
		}
	}
	return false
}

// isLexiconUrl accepts only DJ session urls, so metadata from the server
// cannot point requests anywhere else.
func isLexiconUrl(u string) bool {
	return strings.HasPrefix(u, "hm://lexicon-session-provider/") && !strings.Contains(u, "..")
}
