//go:build preview

package ui

// Renders sample screens to $PREVIEW_DIR for visual checks without a
// Spotify connection:  PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/devgianlu/go-librespot/daemon"
	"github.com/muesli/termenv"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/viz"
	"github.com/Nikkoro/spotui/internal/webapi"
)

func sampleCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 640, 640))
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			dx, dy := float64(x-320), float64(y-300)
			d := math.Hypot(dx, dy)
			switch {
			case d < 140:
				img.Set(x, y, color.RGBA{250, 120, 60, 255})
			case d < 150:
				img.Set(x, y, color.RGBA{255, 200, 120, 255})
			case y > 470:
				img.Set(x, y, color.RGBA{20, uint8(30 + (y-470)/3), 70, 255})
			default:
				img.Set(x, y, color.RGBA{uint8(60 + y/6), 30, uint8(90 + x/8), 255})
			}
		}
	}
	return img
}

func sampleModel(t *testing.T) Model {
	cfg := config.Default()
	cfg.DeviceName = "spotui (desktop)"
	m := New(nil, &cfg, viz.NewTap())
	m.width, m.height = 120, 36
	m.phase = phaseReady
	m.username = "niko"
	cover := "https://example/cover.jpg"
	m.track = &daemon.ApiTrack{
		Name: "Midnight City", ArtistNames: []string{"M83"}, AlbumName: "Hurry Up, We're Dreaming",
		ReleaseDate: "2011-10-18", Duration: 243000, Uri: "spotify:track:1", AlbumCoverUrl: &cover,
	}
	m.coverURL = cover
	m.coverImg = sampleCover()
	m.repixelate()
	m.paused = false
	m.stopped = false
	m.posMs = 97000
	m.posAt = time.Now()
	m.volume = 72
	m.shuffle = true
	m.repeatCtx = true
	m.liked["spotify:track:1"] = true

	// Fake a spectrum: a rough pink-noise slope with some bumps.
	_ = m.View() // lets the visualizer size its bands
	n := m.vizAn.Bands()
	m.vizLevels, m.vizPeaks = make([]float64, n), make([]float64, n)
	for i := range m.vizLevels {
		x := float64(i) / float64(n)
		m.vizLevels[i] = math.Max(0.05, 0.9-0.6*x+0.25*math.Sin(x*17))
		m.vizPeaks[i] = math.Min(1, m.vizLevels[i]+0.12)
	}
	return m
}

func write(t *testing.T, name string, m Model) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("PREVIEW_DIR not set")
	}
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, name+".ans"), []byte(m.View()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreview(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	m := sampleModel(t)
	write(t, "1-now-playing", m)

	m.page = pageLibrary
	var items []listItem
	items = append(items, listItem{kind: kindPlaylist, title: "♥ Liked Songs", subtitle: "your saved tracks", uri: "spotify:user:niko:collection"})
	for _, n := range []string{"Synthwave Nights", "Polish Hip-Hop", "Deep Focus", "Gym 🔥", "Road Trip 2026", "Chill Sunday", "Discover Weekly", "Release Radar", "Lo-fi Beats", "Throwbacks"} {
		items = append(items, listItem{kind: kindPlaylist, title: n, subtitle: "yours", right: "42 tracks", uri: "spotify:playlist:" + n})
	}
	m.lib.lists[0].setItems(items)
	m.lib.loaded[0] = true
	m.lib.lists[0].move(2)
	write(t, "2-library", m)

	m.page = pageContext
	cp := &contextPage{uri: "spotify:album:x", title: "Hurry Up, We're Dreaming", kind: "Album", length: 22, cached: 22, ready: true}
	cp.list.numbered = true
	var tracks []listItem
	for i, n := range []string{"Intro", "Midnight City", "Reunion", "Where the Boats Go", "Wait", "Raconte-moi une histoire", "Train to Pluton", "Claudia Lewis", "This Bright Flash", "When Will You Come Home?", "Soon, My Friend", "My Tears Are Becoming a Sea"} {
		uri := "spotify:track:x" + itoa(i)
		if n == "Midnight City" {
			uri = "spotify:track:1"
		}
		tracks = append(tracks, listItem{kind: kindTrack, title: n, subtitle: "M83", right: "4:03", uri: uri, context: cp.uri})
	}
	cp.list.setItems(tracks)
	cp.list.move(3)
	m.ctxPage = cp
	m.contextURI = cp.uri
	write(t, "3-album", m)

	m.page = pageSearch
	m.search.query = "m83"
	m.setSearchResults(&webapi.SearchResult{
		Tracks: []webapi.Track{
			{Named: webapi.Named{URI: "spotify:track:1", Name: "Midnight City"}, DurationMs: 243000, Artists: []webapi.Named{{Name: "M83"}}, Album: webapi.Album{Named: webapi.Named{Name: "Hurry Up, We're Dreaming"}}},
			{Named: webapi.Named{URI: "spotify:track:2", Name: "Wait"}, DurationMs: 343000, Artists: []webapi.Named{{Name: "M83"}}, Album: webapi.Album{Named: webapi.Named{Name: "Hurry Up, We're Dreaming"}}},
		},
		Albums: []webapi.Album{{Named: webapi.Named{URI: "a", Name: "Saturdays = Youth"}, ReleaseDate: "2008"}},
	})
	write(t, "4-search", m)

	m.page = pageQueue
	m.queue.list.setItems([]listItem{
		{kind: kindHeader, title: "Now playing"},
		{kind: kindTrack, title: "Midnight City", subtitle: "M83", right: "4:03", uri: "spotify:track:1"},
		{kind: kindHeader, title: "Next up"},
		{kind: kindTrack, title: "Reunion", subtitle: "M83", right: "queued  3:55", uri: "q1"},
		{kind: kindTrack, title: "Where the Boats Go", subtitle: "M83", right: "1:59", uri: "q2"},
	})
	m.queue.list.move(1)
	write(t, "5-queue", m)

	auth := m
	auth.phase = phaseAuth
	auth.auth = &daemon.ApiDeviceAuth{Url: "https://www.spotify.com/pair?code=ABCD-EFGH", Code: "ABCD-EFGH", ExpiresAt: time.Now().Add(9 * time.Minute)}
	write(t, "6-auth", auth)

	small := sampleModel(t)
	small.width, small.height = 80, 24
	write(t, "7-now-playing-small", small)

	tall := sampleModel(t)
	tall.width, tall.height = 100, 44
	_ = tall.View()
	n := tall.vizAn.Bands()
	tall.vizLevels, tall.vizPeaks = make([]float64, n), make([]float64, n)
	for i := range tall.vizLevels {
		x := float64(i) / float64(n)
		tall.vizLevels[i] = math.Max(0.05, 0.9-0.6*x+0.25*math.Sin(x*17))
		tall.vizPeaks[i] = math.Min(1, tall.vizLevels[i]+0.12)
	}
	write(t, "8-now-playing-tall", tall)

	var _ tea.Model = m
}
