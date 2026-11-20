package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// uiState is what the interface remembers between runs.
type uiState struct {
	Page          int      `json:"page"`
	LibraryTab    int      `json:"library_tab"`
	PixelSize     int      `json:"pixel_size"`
	Levels        *int     `json:"levels"`
	Visualizer    *bool    `json:"visualizer"`
	VizStyle      string   `json:"viz_style"`
	SearchHistory []string `json:"search_history"`
	Theme         string   `json:"theme,omitempty"`
	ImageMode     *bool    `json:"image_mode,omitempty"`
	ImagePixels   *int     `json:"image_pixels,omitempty"`
	ImageLevels   *int     `json:"image_levels,omitempty"`
	DynamicAccent *bool    `json:"dynamic_accent,omitempty"`
}

const maxSearchHistory = 30

func (m Model) statePath() string {
	if m.cfg == nil || m.cfg.ConfigDir == "" {
		return ""
	}
	return filepath.Join(m.cfg.ConfigDir, "ui-state.json")
}

// restoreState applies the remembered state over the config defaults.
func (m *Model) restoreState() {
	p := m.statePath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var s uiState
	if json.Unmarshal(b, &s) != nil {
		return
	}
	if s.Page >= 0 && s.Page < len(pageNames) && page(s.Page) != pageSearch {
		m.page = page(s.Page)
	}
	if s.LibraryTab >= 0 && s.LibraryTab < len(libTabs) {
		m.lib.tab = s.LibraryTab
	}
	if s.PixelSize > 0 {
		m.pixSize = clamp(s.PixelSize, minPixelSize, maxPixelSize)
	}
	if s.Levels != nil {
		m.levels = *s.Levels
	}
	if s.Visualizer != nil && m.vizTap != nil {
		m.vizOn = *s.Visualizer
	}
	if s.VizStyle != "" {
		m.vizStyle = s.VizStyle
	}
	m.searchHistory = s.SearchHistory
	// A theme picked in the palette wins over config.toml's default.
	if s.Theme != "" && ApplyPalette(s.Theme) {
		m.themeName = s.Theme
	}
	if s.ImagePixels != nil {
		m.imgPixels = *s.ImagePixels
	}
	if s.ImageLevels != nil {
		m.imgLevels = *s.ImageLevels
	}
	m.savedImageMode = s.ImageMode
	if s.DynamicAccent != nil {
		m.dynamicAccent = *s.DynamicAccent
	}
	m.applyAccent()
}

// SaveState remembers pages, look settings and search history for next time.
func (m Model) SaveState() error {
	p := m.statePath()
	if p == "" {
		return nil
	}
	pg := m.page
	if pg == pageContext {
		pg = pageLibrary
	}
	levels, viz, dyn, imgMode, imgPx, imgLv := m.levels, m.vizOn, m.dynamicAccent, m.imageMode, m.imgPixels, m.imgLevels
	b, err := json.MarshalIndent(uiState{
		Page:          int(pg),
		LibraryTab:    m.lib.tab,
		PixelSize:     m.pixSize,
		Levels:        &levels,
		Visualizer:    &viz,
		VizStyle:      m.vizStyle,
		SearchHistory: m.searchHistory,
		Theme:         m.themeName,
		ImageMode:     &imgMode,
		ImagePixels:   &imgPx,
		ImageLevels:   &imgLv,
		DynamicAccent: &dyn,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func (m *Model) rememberSearch(q string) {
	h := []string{q}
	for _, old := range m.searchHistory {
		if old != q && len(h) < maxSearchHistory {
			h = append(h, old)
		}
	}
	m.searchHistory = h
	m.historyIdx = -1
}
