package ui

import "testing"

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
