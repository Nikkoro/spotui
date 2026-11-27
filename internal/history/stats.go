package history

import (
	"slices"
	"strings"
	"time"
)

// Totals are what was listened to in some span.
type Totals struct {
	Ms     int64 // time listened
	Plays  int
	Tracks int // different tracks
}

// Count is one track or artist with how much it was played.
type Count struct {
	Key     string // URI for a track, the name for an artist
	Title   string
	Artists []string
	Album   string
	Plays   int
	Ms      int64
}

// Stats sum up the history at a moment.
type Stats struct {
	Today, Week, Month, All Totals
	// Streak is how many days in a row up to today (or yesterday, if
	// nothing played yet today) had something played.
	Streak int
	// TopTracks and TopArtists are over the last 30 days, most played
	// first; PeakHour is the hour of the day with most listening then
	// (-1 with no plays).
	TopTracks  []Count
	TopArtists []Count
	PeakHour   int
}

// Summarize sums plays up as of now, in now's time zone; top lists keep n
// entries.
func Summarize(plays []Play, now time.Time, n int) Stats {
	loc := now.Location()
	day := func(t time.Time) time.Time {
		t = t.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	}
	today := day(now)
	since := map[*Totals]time.Time{}
	s := Stats{PeakHour: -1}
	since[&s.Today] = today
	since[&s.Week] = today.AddDate(0, 0, -6)
	since[&s.Month] = today.AddDate(0, 0, -29)
	since[&s.All] = time.Time{}
	seen := map[*Totals]map[string]bool{}

	tracks := map[string]*Count{}
	artists := map[string]*Count{}
	var hours [24]int64
	days := map[time.Time]bool{}
	for _, p := range plays {
		d := day(p.At)
		days[d] = true
		for t, from := range since {
			if !p.At.Before(from) {
				t.Ms += p.ListenedMs
				t.Plays++
				if seen[t] == nil {
					seen[t] = map[string]bool{}
				}
				if !seen[t][p.URI] {
					seen[t][p.URI] = true
					t.Tracks++
				}
			}
		}
		if p.At.Before(since[&s.Month]) {
			continue
		}
		hours[p.At.In(loc).Hour()] += p.ListenedMs
		c := tracks[p.URI]
		if c == nil {
			c = &Count{Key: p.URI, Title: p.Title, Artists: p.Artists, Album: p.Album}
			tracks[p.URI] = c
		}
		c.Plays++
		c.Ms += p.ListenedMs
		for _, a := range p.Artists {
			ac := artists[a]
			if ac == nil {
				ac = &Count{Key: a, Title: a}
				artists[a] = ac
			}
			ac.Plays++
			ac.Ms += p.ListenedMs
		}
	}

	for d := today; days[d]; d = d.AddDate(0, 0, -1) {
		s.Streak++
	}
	if s.Streak == 0 {
		for d := today.AddDate(0, 0, -1); days[d]; d = d.AddDate(0, 0, -1) {
			s.Streak++
		}
	}
	var best int64
	for h, ms := range hours {
		if ms > best {
			best, s.PeakHour = ms, h
		}
	}
	s.TopTracks = top(tracks, n)
	s.TopArtists = top(artists, n)
	return s
}

func top(m map[string]*Count, n int) []Count {
	out := make([]Count, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Count) int {
		switch {
		case a.Plays != b.Plays:
			return b.Plays - a.Plays
		case a.Ms != b.Ms:
			return int(b.Ms - a.Ms)
		}
		return strings.Compare(a.Title, b.Title)
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
