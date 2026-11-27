package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The library is cached on disk and shown at once on the next start while a
// fresh copy loads, so big libraries do not mean waiting at an empty list.

type cachedItem struct {
	Kind     itemKind `json:"k"`
	Title    string   `json:"t"`
	Subtitle string   `json:"s,omitempty"`
	Right    string   `json:"r,omitempty"`
	URI      string   `json:"u"`
	Image    string   `json:"i,omitempty"`
	Editable bool     `json:"e,omitempty"`
	Owned    bool     `json:"o,omitempty"`
	Desc     string   `json:"d,omitempty"`
}

func (m Model) libCachePath(tab int) string {
	if m.cfg == nil || m.cfg.CacheDir == "" || m.username == "" || tab == libStats {
		return "" // Stats are read from the local history, never cached
	}
	// Per account, so switching accounts never shows someone else's library.
	// By the tab's name, so adding a tab never shows another tab's list.
	name := strings.ReplaceAll(strings.ToLower(libTabs[tab]), " ", "-")
	return filepath.Join(m.cfg.CacheDir, "library", m.username+"-"+name+".json")
}

func (m Model) readLibCache(tab int) []listItem {
	p := m.libCachePath(tab)
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var cached []cachedItem
	if json.Unmarshal(b, &cached) != nil {
		return nil
	}
	items := make([]listItem, len(cached))
	for i, c := range cached {
		items[i] = listItem{kind: c.Kind, title: c.Title, subtitle: c.Subtitle, right: c.Right, uri: c.URI, imageURL: c.Image, editable: c.Editable, owned: c.Owned, desc: c.Desc}
	}
	return items
}

func (m Model) writeLibCache(tab int, items []listItem) {
	p := m.libCachePath(tab)
	if p == "" {
		return
	}
	cached := make([]cachedItem, len(items))
	for i, it := range items {
		cached[i] = cachedItem{Kind: it.kind, Title: it.title, Subtitle: it.subtitle, Right: it.right, URI: it.uri, Image: it.imageURL, Editable: it.editable, Owned: it.owned, Desc: it.desc}
	}
	b, err := json.Marshal(cached)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o700) == nil {
		_ = os.WriteFile(p, b, 0o600)
	}
}
