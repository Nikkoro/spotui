package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func sigEscape() string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm ", shaderSig[0], shaderSig[1], shaderSig[2])
}

func TestShaderStripOnlyWhenAskedAndLyricsShow(t *testing.T) {
	m := fxModel(t, fxOff)
	first := func(m Model) string { l, _, _ := strings.Cut(m.View(), "\n"); return l }

	if strings.Contains(first(m), sigEscape()) {
		t.Fatal("strip drawn with wt_shader off")
	}
	m.cfg.Lyrics.WTShader = true
	row := first(m)
	if !strings.HasPrefix(row, sigEscape()) || strings.Count(row, sigEscape()) != 2 {
		t.Fatalf("no strip at the start of the top row: %q", row)
	}
	if w := lipgloss.Width(row); w != m.width {
		t.Fatalf("top row %d wide, want %d", w, m.width)
	}
	m.page = pageQueue
	if strings.Contains(first(m), sigEscape()) {
		t.Fatal("strip drawn without lyrics on screen")
	}
}

func TestShaderGetsTheBeatWithEffectsOff(t *testing.T) {
	m := fxModel(t, fxOff)
	m.cfg.Lyrics.WTShader = true
	dt := 1.0 / 30
	for i := 0; i < 40; i++ {
		m.stepFx(kick(0.02), dt)
	}
	m.stepFx(kick(0.8), dt)
	if m.fx.kick < 0.5 || m.fx.level == 0 {
		t.Fatalf("shader not fed: kick %.2f, level %.2f", m.fx.kick, m.fx.level)
	}
	if m.fx.pulse != 0 || len(m.fx.sparks) != 0 {
		t.Fatal("effects off but the line moved")
	}
}

func TestShaderSignatureMatchesTheHLSL(t *testing.T) {
	src, err := os.ReadFile("../../shaders/spotui.hlsl")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("float3(%d.0, %d.0, %d.0) / 255.0", shaderSig[0], shaderSig[1], shaderSig[2])
	if !strings.Contains(string(src), want) {
		t.Fatalf("shaders/spotui.hlsl does not use the signature %v", shaderSig)
	}
	if !strings.Contains(string(src), fmt.Sprintf("STRIP_CELLS = %d;", shaderCells)) {
		t.Fatalf("shaders/spotui.hlsl does not expect %d cells", shaderCells)
	}
}
