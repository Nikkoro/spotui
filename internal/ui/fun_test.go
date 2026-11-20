package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/viz"
)

func funModel() Model {
	cfg := config.Default()
	m := New(nil, &cfg, viz.NewTap())
	m.width, m.height = 100, 30
	return m
}

func TestProgressStylesKeepWidth(t *testing.T) {
	m := funModel()
	for _, s := range progressStyles {
		m.cfg.UI.ProgressStyle = s
		bar := m.progressBar(40, 60000, 200000)
		if w := lipgloss.Width(bar); w != 40 {
			t.Errorf("%s: %d wide, want 40", s, w)
		}
		if !strings.Contains(ansi.Strip(bar), "●") {
			t.Errorf("%s: no knob", s)
		}
	}
}

func TestSpotifyURL(t *testing.T) {
	if got := spotifyURL("spotify:track:4uLU6hMCjMI75M1A2tKUQC"); got != "https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC" {
		t.Fatal(got)
	}
	if spotifyURL("spotify:user:x:collection") != "" {
		t.Fatal("a collection has no simple link")
	}
}

func TestHeartSparklesThenSettles(t *testing.T) {
	m := funModel()
	m.liked["u"] = true
	m.likedNowURI, m.likedNowAt = "u", time.Now().Add(-200*time.Millisecond)
	if got := ansi.Strip(m.heart("u", "")); got == "♥" {
		t.Fatal("no sparkle right after liking")
	}
	m.likedNowAt = time.Now().Add(-2 * time.Second)
	if got := ansi.Strip(m.heart("u", "")); got != "♥" {
		t.Fatalf("still sparkling: %q", got)
	}
	m.cfg.UI.LikeAnimation = false
	m.likedNowAt = time.Now()
	if got := ansi.Strip(m.heart("u", "")); got != "♥" {
		t.Fatalf("animation off but sparkling: %q", got)
	}
}

func TestSplash(t *testing.T) {
	m := funModel()
	if out := ansi.Strip(m.splash("Connecting…")); !strings.Contains(out, "spotui") || !strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Fatalf("splash: %q", out)
	}
	m.cfg.UI.Splash = false
	if out := ansi.Strip(m.splash("Connecting…")); strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Fatal("splash off but animated")
	}
}
