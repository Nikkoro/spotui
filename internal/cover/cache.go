package cover

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Cache keeps downloaded cover images on disk, so covers show instantly on
// tracks played before and survive offline. Images are stored as fetched
// (JPEG from Spotify's CDN), keyed by URL.
type Cache struct {
	dir      string
	maxBytes int64

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done chan struct{}
	img  image.Image
	err  error
}

// NewCache stores covers in dir, keeping at most maxBytes (0: 200 MB).
func NewCache(dir string, maxBytes int64) *Cache {
	if maxBytes <= 0 {
		maxBytes = 200 << 20
	}
	return &Cache{dir: dir, maxBytes: maxBytes, inflight: map[string]*call{}}
}

func (c *Cache) path(url string) string {
	sum := sha1.Sum([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:]))
}

// File is where the cover for url lies on disk (as fetched, usually
// JPEG), or "" while it is not there.
func (c *Cache) File(url string) string {
	if url == "" {
		return ""
	}
	p := c.path(url)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// Get returns the cover for url from disk, or downloads and stores it.
// Concurrent requests for the same URL share one download.
func (c *Cache) Get(ctx context.Context, url string) (image.Image, error) {
	if url == "" {
		return nil, errors.New("no cover")
	}
	p := c.path(url)
	if b, err := os.ReadFile(p); err == nil {
		if img, _, err := image.Decode(bytes.NewReader(b)); err == nil {
			now := time.Now()
			_ = os.Chtimes(p, now, now) // keeps eviction least-recently-used
			return img, nil
		}
		_ = os.Remove(p) // corrupt entry
	}

	c.mu.Lock()
	if cl, ok := c.inflight[url]; ok {
		c.mu.Unlock()
		select {
		case <-cl.done:
			return cl.img, cl.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	cl := &call{done: make(chan struct{})}
	c.inflight[url] = cl
	c.mu.Unlock()

	cl.img, cl.err = c.download(ctx, url, p)
	close(cl.done)

	c.mu.Lock()
	delete(c.inflight, url)
	c.mu.Unlock()
	return cl.img, cl.err
}

// Prefetch warms the cache for url in the background.
func (c *Cache) Prefetch(url string) {
	if url == "" {
		return
	}
	if _, err := os.Stat(c.path(url)); err == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = c.Get(ctx, url)
	}()
}

func (c *Cache) download(ctx context.Context, url, p string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decoding cover: %w", err)
	}

	if err := os.MkdirAll(c.dir, 0o700); err == nil {
		tmp := p + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, p)
		}
	}
	return img, nil
}

// Prune deletes the least recently used covers beyond the size limit.
func (c *Cache) Prune() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, file{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= c.maxBytes*9/10 { // leave headroom so we do not prune every run
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
