package webapi

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Podcasts: saved shows, a show's episodes and an episode's description,
// read as spotify_player does. Progress (played, minutes left) comes from
// the player's resumption service instead, which needs no extra scope.

const (
	ttlShows    = 10 * time.Minute // the saved shows
	ttlShow     = time.Hour        // a show's details
	ttlEpisodes = 10 * time.Minute // a page of a show's episodes
	ttlEpisode  = 24 * time.Hour   // an episode's description

	episodesPage = 50
)

// Show is a podcast.
type Show struct {
	Named
	Publisher     string  `json:"publisher"`
	Description   string  `json:"description"`
	Images        []Image `json:"images"`
	TotalEpisodes int     `json:"total_episodes"`
}

// Episode is a podcast episode.
type Episode struct {
	Named
	Description string `json:"description"`
	// HTMLDescription keeps the paragraphs and line breaks Description
	// drops.
	HTMLDescription string  `json:"html_description"`
	ReleaseDate     string  `json:"release_date"`
	DurationMs      int     `json:"duration_ms"`
	Images          []Image `json:"images"`
	// Show is set on a single episode and in search results.
	Show *Show `json:"show,omitempty"`
}

// SavedShows returns the podcasts the account follows, newest first.
func (c *Client) SavedShows(ctx context.Context) ([]Show, error) {
	var all []Show
	for offset := 0; offset < 1000; offset += 50 {
		var raw page[struct {
			Show Show `json:"show"`
		}]
		q := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}}
		if err := c.get(ctx, "/me/shows", q, ttlShows, &raw); err != nil {
			return nil, err
		}
		for _, it := range raw.Items {
			if it != nil {
				all = append(all, it.Show)
			}
		}
		if len(raw.Items) < 50 || len(all) >= raw.Total {
			break
		}
	}
	return all, nil
}

// ShowDetails returns a show's name, publisher and description.
func (c *Client) ShowDetails(ctx context.Context, uri string) (*Show, error) {
	var s Show
	if err := c.get(ctx, "/shows/"+idOf(uri), nil, ttlShow, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ShowEpisodes returns a page of a show's episodes, newest first, and how
// many there are in all.
func (c *Client) ShowEpisodes(ctx context.Context, uri string, offset int) ([]Episode, int, error) {
	var raw page[Episode]
	q := url.Values{"limit": {strconv.Itoa(episodesPage)}, "offset": {strconv.Itoa(offset)}}
	if err := c.get(ctx, "/shows/"+idOf(uri)+"/episodes", q, ttlEpisodes, &raw); err != nil {
		return nil, 0, err
	}
	return compact(raw.Items), raw.Total, nil
}

// EpisodeDetails returns one episode with its description and show.
func (c *Client) EpisodeDetails(ctx context.Context, uri string) (*Episode, error) {
	var e Episode
	if err := c.get(ctx, "/episodes/"+idOf(uri), nil, ttlEpisode, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// idOf is the id in spotify:<type>:<id>.
func idOf(uri string) string {
	return uri[strings.LastIndex(uri, ":")+1:]
}
