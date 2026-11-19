package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefaultsAndFlagsTypos(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal("config.toml not created")
	}
	if cfg.Bitrate != 320 || !cfg.Cover.Enabled || cfg.AudioBackend == "" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}

	// Written defaults must load back cleanly.
	again, err := Load(dir)
	if err != nil || len(again.Warnings) != 0 {
		t.Fatalf("reloading defaults: %v %v", err, again.Warnings)
	}

	_ = os.WriteFile(filepath.Join(dir, "config.toml"), []byte(
		"bitrate = 160\nbitrat = 96\n[cover]\npixel_sise = 20\n[keys]\nnext = \"ctrl+n\"\n[colors]\naccent = \"#ffffff\"\n"), 0o600)
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bitrate != 160 || cfg.Keys["next"] != "ctrl+n" || cfg.Colors["accent"] != "#ffffff" {
		t.Fatalf("values not read: %+v", cfg)
	}
	if len(cfg.Warnings) != 2 {
		t.Fatalf("want 2 typo warnings, got %v", cfg.Warnings)
	}
	if cfg.InitialVolume != 70 {
		t.Fatal("missing keys must keep their defaults")
	}
}
