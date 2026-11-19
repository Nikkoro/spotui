package ui

import (
	"strings"
	"testing"

	"github.com/Nikkoro/spotui/internal/config"
)

func TestParseSpotifyURI(t *testing.T) {
	cases := map[string]string{
		"spotify:track:4uLU6hMCjMI75M1A2tKUQC":                          "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
		"https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC?si=abc":  "spotify:track:4uLU6hMCjMI75M1A2tKUQC",
		"https://open.spotify.com/intl-pl/album/1DFixLWuPkv3KT3TnV35m3": "spotify:album:1DFixLWuPkv3KT3TnV35m3",
		"  https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M  ":  "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
		"spotify:user:someone:collection":                               "spotify:user:someone:collection",
	}
	for in, want := range cases {
		got, err := parseSpotifyURI(in)
		if err != nil || got != want {
			t.Errorf("parseSpotifyURI(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	for _, bad := range []string{"", "hello", "https://example.com/track/x", "https://open.spotify.com/user/x", "spotify:"} {
		if got, err := parseSpotifyURI(bad); err == nil {
			t.Errorf("parseSpotifyURI(%q) = %q, want error", bad, got)
		}
	}
}

func TestTabAtMatchesHeader(t *testing.T) {
	cfg := config.Default()
	m := Model{theme: newTheme(defaultAccent), phase: phaseReady, cfg: &cfg}
	m.width = 140
	// Find each tab's label in the rendered header and click its middle.
	header := stripANSI(m.viewHeader(m.width - 4))
	for i, name := range pageNames {
		label := itoa(i+1) + " " + name
		idx := indexRunes(header, label)
		if idx < 0 {
			t.Fatalf("label %q not in header %q", label, header)
		}
		p, ok := m.tabAt(2+idx+len([]rune(label))/2, 1)
		if !ok || p != page(i) {
			t.Errorf("click on %q gave %v %v", label, p, ok)
		}
	}
}

func stripANSI(s string) string {
	out := []rune{}
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			out = append(out, r)
		}
	}
	return string(out)
}

func indexRunes(s, sub string) int {
	rs, subr := []rune(s), []rune(sub)
	for i := 0; i+len(subr) <= len(rs); i++ {
		if string(rs[i:i+len(subr)]) == sub {
			return i
		}
	}
	return -1
}

func TestListScrollPersistsAcrossCopies(t *testing.T) {
	var items []listItem
	for i := 0; i < 100; i++ {
		items = append(items, listItem{title: itoa(i), uri: itoa(i)})
	}
	type holder struct{ l listModel }
	h := holder{}
	h.l.setItems(items)
	th := newTheme(defaultAccent)

	h.l.move(50)
	view := func() string { c := h; return c.l.view(th, 40, 10, "") } // View runs on a copy
	view()
	// Scrolling back up a little must keep the window, not jump to the top.
	h.l.move(-3)
	out := stripANSI(view())
	if !strings.Contains(out, " 47") || strings.Contains(out, " 0\n") {
		t.Fatalf("window lost its position:\n%s", out)
	}
	first := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	if first == "0" {
		t.Fatalf("window jumped to the top")
	}
}
