package ui

import (
	"encoding/json"
	"os"
	"testing"
)

// A run in a terminal without images (tmux) must not turn images off for
// the terminal that has them.
func TestSaveStateKeepsImageChoiceWithoutImages(t *testing.T) {
	m := remoteModel(t) // no image writer: blocks only
	on := true
	m.savedImageMode = &on
	if err := m.SaveState(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(m.statePath())
	if err != nil {
		t.Fatal(err)
	}
	var s uiState
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.ImageMode == nil || !*s.ImageMode {
		t.Fatalf("image_mode overwritten: %s", b)
	}
}
