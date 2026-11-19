package ui

import (
	"testing"
	"time"
)

func TestKeymap(t *testing.T) {
	remap, unknown := buildKeymap(map[string]string{
		"next":       "ctrl+n, L",
		"play_pause": "k",
		"bogus":      "x",
		"quit":       "q", // same as default: no entry
	})
	if len(unknown) != 1 || unknown[0] != "bogus" {
		t.Fatalf("unknown = %v", unknown)
	}
	want := map[string]string{"ctrl+n": "n", "L": "n", "k": " "}
	if len(remap) != len(want) {
		t.Fatalf("remap = %v", remap)
	}
	for k, v := range want {
		if remap[k] != v {
			t.Errorf("remap[%q] = %q, want %q", k, remap[k], v)
		}
	}
	// The produced events must read back as the default keys.
	for _, def := range []string{" ", "n", "right", "enter", "?"} {
		if got := keyMsg(def).String(); got != def {
			t.Errorf("keyMsg(%q).String() = %q", def, got)
		}
	}
}

func TestPaletteSearchEntryOnce(t *testing.T) {
	m := Model{pal: newPalette()}
	m.pal.cmds = []paletteCmd{{title: "Search", key: "3"}, {title: "Queue", key: "4"}}
	m.setPaletteItems("search")
	n := 0
	for _, v := range m.pal.list.visible {
		if m.pal.list.items[v].uri == "palette:search" {
			n++
		}
	}
	if n != 1 || m.pal.list.visible[0] != 0 {
		t.Fatalf("search entry shown %d times, visible=%v", n, m.pal.list.visible)
	}
}

func TestColorOverridesSurvivePaletteSwitch(t *testing.T) {
	defer func() { SetColorOverrides(nil); ApplyPalette("spotify") }()
	bad := SetColorOverrides(map[string]string{"accent": "#ff00aa", "text": "nope", "glow": "#000000"})
	if len(bad) != 2 {
		t.Fatalf("bad = %v", bad)
	}
	ApplyPalette("nord")
	if defaultAccent != "#ff00aa" {
		t.Fatalf("accent %s, override lost on palette switch", defaultAccent)
	}
}

func TestAgoLabel(t *testing.T) {
	now := time.Now()
	cases := map[time.Duration]string{10 * time.Second: "1m ago", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago", 72 * time.Hour: "3d ago"}
	for d, want := range cases {
		if got := agoLabel(now.Add(-d).Format(time.RFC3339)); got != want {
			t.Errorf("%s ago: got %q want %q", d, got, want)
		}
	}
	if agoLabel("garbage") != "" {
		t.Error("bad time must give empty label")
	}
}
