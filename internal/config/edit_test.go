package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

const sample = `# spotui configuration
device_name = "old"   # shown in Spotify Connect
bitrate = 320

[cover]
enabled = true
# pixels per side
image_pixels = 64

[visualizer]
style = "bars"
`

func TestSetInText(t *testing.T) {
	got := setInText(sample, "", "device_name", `"spotui (WSL)"`)
	got = setInText(got, "cover", "image_pixels", "32")
	got = setInText(got, "visualizer", "delay_ms", "200")
	got = setInText(got, "", "crossfade_ms", "6000")
	got = setInText(got, "update", "prerelease", "true")

	var cfg Config
	if _, err := toml.Decode(got, &cfg); err != nil {
		t.Fatalf("not valid TOML: %v\n%s", err, got)
	}
	if cfg.DeviceName != "spotui (WSL)" || cfg.Cover.ImagePixels != 32 || cfg.Visualizer.DelayMs != 200 ||
		cfg.CrossfadeMs != 6000 || !cfg.Update.Prerelease || cfg.Bitrate != 320 || cfg.Visualizer.Style != "bars" {
		t.Fatalf("values not set:\n%s", got)
	}
	for _, keep := range []string{"# spotui configuration", "# shown in Spotify Connect", "# pixels per side"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q:\n%s", keep, got)
		}
	}
}

func TestSetKeyWritesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetKey(p, "visualizer", "style", `wave "x"`); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if _, err := toml.DecodeFile(p, &cfg); err != nil || cfg.Visualizer.Style != `wave "x"` {
		t.Fatalf("%v %q", err, cfg.Visualizer.Style)
	}
}

func TestDefaultsTurnOnChecksAndTouches(t *testing.T) {
	d := Default()
	if !d.Update.Check || !d.UI.Splash || !d.UI.LikeAnimation || d.UI.ProgressStyle == "" || d.UI.CopyAs == "" {
		t.Fatalf("defaults missing: %+v %+v", d.Update, d.UI)
	}
}
