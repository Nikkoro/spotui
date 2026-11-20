package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/viz"
)

func TestSettingsChangeAndSave(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, cfg, viz.NewTap())
	m.width, m.height = 120, 40
	m.phase = phaseReady
	_ = m.openSettings()

	find := func(label string) setting {
		for _, s := range m.settings() {
			if s.label == label {
				return s
			}
		}
		t.Fatalf("no setting %q", label)
		return setting{}
	}
	_ = m.stepSetting(find("Crossfade"), +1)
	_ = m.stepSetting(find("Release channel"), +1)
	_ = m.changeSetting(find("Device name"), "spotui (WSL)")

	if m.cfg.CrossfadeMs != 1000 || !m.cfg.Update.Prerelease || m.cfg.DeviceName != "spotui (WSL)" {
		t.Fatalf("not applied: %+v", m.cfg)
	}
	if !m.set.restart {
		t.Fatal("crossfade waits for a restart")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	for _, want := range []string{"crossfade_ms = 1000", "prerelease = true", `device_name = "spotui (WSL)"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("config.toml lacks %q", want)
		}
	}
	reloaded, err := config.Load(dir)
	if err != nil || reloaded.CrossfadeMs != 1000 || !reloaded.Update.Prerelease {
		t.Fatalf("reload: %v %+v", err, reloaded)
	}
	if out := m.viewSettings(120, 40); !strings.Contains(out, "Settings") {
		t.Fatal("settings page did not render")
	}
}
