package shaders

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	if wrote, err := Refresh(path); wrote || err != nil {
		t.Fatalf("refresh of a missing file: %v %v", wrote, err)
	}
	if wrote, err := Install(path); !wrote || err != nil {
		t.Fatalf("install: %v %v", wrote, err)
	}
	if wrote, _ := Install(path); wrote {
		t.Fatal("wrote an unchanged shader again")
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if wrote, err := Refresh(path); !wrote || err != nil {
		t.Fatalf("refresh of an old shader: %v %v", wrote, err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(Source) {
		t.Fatal("shader not brought up to date")
	}
}
