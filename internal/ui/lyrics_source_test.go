package ui

import (
	"strings"
	"testing"
)

func TestLyricsShapeLeavesTheTextOut(t *testing.T) {
	// Invented lyrics, shaped like a color-lyrics response.
	raw := `{"lyrics":{"syncType":"LINE_SYNCED","language":"en","lines":[{"startTimeMs":"1000","words":"paper boats on a quiet street"}],
		"alternatives":[{"language":"fr","lines":["des bateaux de papier"],"isRtlLanguage":false},
		{"language":"de","lines":["Papierboote"]}]},"colors":{"background":-1}}`
	got := lyricsShape([]byte(raw))
	for _, want := range []string{"keys colors,lyrics", "language en", "alternatives 2:", "[fr 1 lines, keys isRtlLanguage,language,lines]", "[de 1 lines"} {
		if !strings.Contains(got, want) {
			t.Errorf("shape %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "paper") || strings.Contains(got, "bateaux") {
		t.Fatalf("the text leaked into the log: %q", got)
	}
	if got := lyricsShape([]byte(`{"lyrics":{"lines":[]}}`)); !strings.Contains(got, "no alternatives") {
		t.Fatalf("no alternatives: %q", got)
	}
}
