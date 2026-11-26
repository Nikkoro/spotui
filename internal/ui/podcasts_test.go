package ui

import (
	"testing"

	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/webapi"
)

func TestEpisodeRight(t *testing.T) {
	dur := int64(45 * 60_000)
	for _, c := range []struct {
		st    engine.EpisodeState
		known bool
		want  string
	}{
		{engine.EpisodeState{}, false, "45:00"},
		{engine.EpisodeState{Finished: true}, true, "✓ played"},
		{engine.EpisodeState{PositionMs: 33 * 60_000}, true, "12 min left"},
		{engine.EpisodeState{PositionMs: 33*60_000 + 1}, true, "12 min left"},
	} {
		if got := episodeRight(dur, c.st, c.known); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.st, got, c.want)
		}
	}
}

func TestShowPageFillsAndPages(t *testing.T) {
	m := remoteModel(t)
	_ = m.openContext(listItem{kind: kindShow, title: "Invented Radio", uri: "spotify:show:s1"})
	if m.ctxPage.kind != "Podcast" {
		t.Fatalf("kind %q", m.ctxPage.kind)
	}
	m.episodes["spotify:episode:e2"] = engine.EpisodeState{Uri: "spotify:episode:e2", Finished: true}
	eps := []webapi.Episode{
		{Named: webapi.Named{URI: "spotify:episode:e1", Name: "One"}, ReleaseDate: "2026-11-20", DurationMs: 60_000},
		{Named: webapi.Named{URI: "spotify:episode:e2", Name: "Two"}, ReleaseDate: "2026-11-13", DurationMs: 60_000},
	}
	cmd := m.handleShowPage(showPageMsg{uri: "spotify:show:s1", show: &webapi.Show{Named: webapi.Named{Name: "Invented Radio"}, Publisher: "Nobody", Description: "About things"},
		episodes: eps, total: 3})
	cp := m.ctxPage
	if cmd == nil || len(cp.list.items) != 2 || cp.subtitle != "Nobody" || cp.desc != "About things" || cp.length != 3 {
		t.Fatalf("page: %+v", cp)
	}
	if it := cp.list.items[1]; it.right != "✓ played" || it.context != "spotify:show:s1" {
		t.Fatalf("episode: %+v", it)
	}
	// The cursor at the end asks for the rest.
	cp.list.bottom()
	if m.moreEpisodes() == nil || !cp.loading {
		t.Fatal("no next page")
	}
	_ = m.handleShowPage(showPageMsg{uri: "spotify:show:s1", offset: 2,
		episodes: []webapi.Episode{{Named: webapi.Named{URI: "spotify:episode:e3", Name: "Three"}}}, total: 3})
	if len(cp.list.items) != 3 || m.moreEpisodes() != nil {
		t.Fatalf("after the last page: %d items", len(cp.list.items))
	}
}

func TestMarkPlayedToggles(t *testing.T) {
	m := remoteModel(t)
	m.track = &daemon.ApiTrack{Uri: "spotify:episode:e1", Name: "One", Duration: 60_000}
	if m.toggleEpisodePlayed() == nil || !m.episodes["spotify:episode:e1"].Finished {
		t.Fatal("not marked")
	}
	if m.toggleEpisodePlayed() == nil || m.episodes["spotify:episode:e1"].Finished {
		t.Fatal("not unmarked")
	}
	m.track = &daemon.ApiTrack{Uri: "spotify:track:t1"}
	if m.toggleEpisodePlayed() != nil {
		t.Fatal("marked a song")
	}
}

func TestLikeFlash(t *testing.T) {
	for _, c := range []struct {
		title, uri string
		on         bool
		want       string
	}{
		{"", "spotify:track:1", true, "Added to Liked Songs"},
		{"X", "spotify:track:1", false, "Removed “X” from Liked Songs"},
		{"Ep", "spotify:episode:1", true, "Saved “Ep” to Your Episodes"},
		{"", "spotify:episode:1", false, "Removed from Your Episodes"},
	} {
		if got := likeFlash(c.title, c.uri, c.on); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	in := `<p>Invented talk about <b>games</b> &amp; films.</p><p>00:00 Intro<br/>03:35 News<br>` +
		`Mugs: <a href="https://example.com/mugs">example.com/mugs</a></p>` +
		`<ul><li>Discord: <a href="https://discord.example/x">join us</a></li><li>Two</li></ul>`
	want := "Invented talk about games & films.\n\n00:00 Intro\n03:35 News\nMugs: example.com/mugs\n\n" +
		"• Discord: join us (https://discord.example/x)\n• Two"
	if got := htmlToText(in); got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestParseChapters(t *testing.T) {
	notes := "Invented talk.\n\n00:00 Intro\n03:35 News – more news\n(1:02:10) - Questions\n\nFollow us."
	ch, rest := parseChapters(notes)
	if len(ch) != 3 || ch[1].startMs != 215_000 || ch[1].title != "News – more news" || ch[2].startMs != 3_730_000 {
		t.Fatalf("chapters: %+v", ch)
	}
	if rest != "Invented talk.\n\nFollow us." {
		t.Fatalf("rest: %q", rest)
	}
	if currentChapter(ch, 300_000) != 1 || currentChapter(ch, 0) != 0 {
		t.Fatal("current chapter")
	}
	// One time, or times out of order, are no chapter list.
	for _, n := range []string{"Live at 20:00 every day", "10:00 Late\n05:00 Early"} {
		if ch, _ := parseChapters(n); ch != nil {
			t.Errorf("%q: %+v", n, ch)
		}
	}
}

func TestJumpChapter(t *testing.T) {
	m := remoteModel(t)
	m.track = &daemon.ApiTrack{Uri: "spotify:episode:e1", Duration: 4_000_000}
	m.episodeNotes["spotify:episode:e1"] = "00:00 Intro\n03:35 News\n10:00 End"
	m.paused = true
	m.setPosition(100_000)
	if m.jumpChapter(+1) == nil || m.posMs != 215_000 {
		t.Fatalf("next: %d", m.posMs)
	}
	m.setPosition(220_000) // 5 s into News: back restarts it
	if m.jumpChapter(-1) == nil || m.posMs != 215_000 {
		t.Fatalf("restart: %d", m.posMs)
	}
	if m.jumpChapter(-1) == nil || m.posMs != 0 {
		t.Fatalf("previous: %d", m.posMs)
	}
}
