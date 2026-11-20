//go:build preview

package ui

// Renders sample screens to $PREVIEW_DIR for visual checks without a
// Spotify connection:  PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui

import (
	"errors"
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
	"github.com/Nikkoro/spotui/internal/cover"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/lyrics"
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
	m.lyrics = &lyrics.Lyrics{Source: "LRCLIB", Synced: lyrics.ParseLRC(
		"[01:20.00]I left the lights on by the water\n[01:24.00]to find my way back after dark\n" +
			"[01:30.00]the radio hums an old address\n[01:34.00]Neon harbour, hold me steady\n" +
			"[01:40.00]every wave is calling twice\n[01:44.00]I keep the night inside my pocket\n" +
			"[01:50.00]and spend it slowly, line by line\n[02:00.00]\n[02:10.00]Neon harbour, hold me steady")}

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
	cp.subtitle = "M83 · 2011"
	cp.cover = cover.Pixelate(sampleCover(), ctxCoverPx, 0)
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

	m.page = pageLyrics
	write(t, "9-lyrics", m)

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

	help := sampleModel(t)
	help.overlay = overlayHelp
	write(t, "10-help", help)

	dev := sampleModel(t)
	dev.page = pageLibrary
	dev.overlay = overlayDevices
	dev.devices.list.setItems([]listItem{
		{kind: kindTrack, title: "♪ spotui (desktop)", subtitle: "computer · this terminal · playing", right: "72%", uri: "a"},
		{kind: kindTrack, title: "Pixel 9", subtitle: "smartphone", right: "40%", uri: "b"},
		{kind: kindTrack, title: "Living Room", subtitle: "speaker", right: "25%", uri: "c"},
	})
	dev.devices.list.move(1)
	write(t, "11-devices", dev)

	mirror := sampleModel(t)
	mirror.vizStyle = "mirror"
	write(t, "12-mirror", mirror)

	wave := sampleModel(t)
	wave.vizStyle = "wave"
	wave.vizWaveGain = 1
	for i := 0; i < 882; i++ {
		x := float64(i) / 44100
		wave.vizWave = append(wave.vizWave, float32(0.5*math.Sin(2*math.Pi*110*x)+0.25*math.Sin(2*math.Pi*440*x+1)+0.1*math.Sin(2*math.Pi*1320*x)))
	}
	write(t, "19-wave", wave)

	djm := sampleModel(t)
	djm.contextURI = engine.DjContextURI
	write(t, "14-dj-now-playing", djm)
	_ = djm.djKey()
	djm.setDjSuggestions(djSuggestionsMsg{prompts: []string{"Throwback hits from the 2000s", "Chill electronic for focus", "Upbeat Polish rap", "Something like M83 but newer"}})
	write(t, "15-dj-panel", djm)

	pk := sampleModel(t)
	pk.page = pageLibrary
	pk.lib.lists[libPlaylists].setItems([]listItem{
		{kind: kindPlaylist, title: "Synthwave Nights", subtitle: "yours", uri: "p1", editable: true},
		{kind: kindPlaylist, title: "Deep Focus", subtitle: "yours", uri: "p2", editable: true},
		{kind: kindPlaylist, title: "Road Trip 2026", subtitle: "yours", uri: "p3", editable: true},
		{kind: kindPlaylist, title: "Discover Weekly", subtitle: "by spotify", uri: "p4"},
	})
	pk.lib.loaded[libPlaylists] = true
	_ = pk.openPlaylistPicker()
	_ = pk.cycleSleep()
	write(t, "16-playlist-picker", pk)

	fatal := sampleModel(t)
	fatal.phase = phaseFatal
	fatal.fatal = errors.New("session lost: connection reset by peer")
	_ = fatal.scheduleRestart()
	write(t, "17-fatal", fatal)

	pal := sampleModel(t)
	pal.lib = m.lib
	_ = pal.openPalette()
	pal.pal.input.SetValue("syn")
	pal.setPaletteItems("syn")
	write(t, "18-palette", pal)

	ApplyPalette("catppuccin")
	cat := sampleModel(t)
	cat.dynamicAccent = false
	cat.repixelate()
	cat.page = pageLibrary
	cat.lib = m.lib
	write(t, "13-catppuccin", cat)
	ApplyPalette("spotify")

	var _ tea.Model = m
}
