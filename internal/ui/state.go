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
	levels, viz := m.levels, m.vizOn
	b, err := json.MarshalIndent(uiState{
		Page:          int(pg),
		LibraryTab:    m.lib.tab,
		PixelSize:     m.pixSize,
		Levels:        &levels,
		Visualizer:    &viz,
		VizStyle:      m.vizStyle,
		SearchHistory: m.searchHistory,
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
