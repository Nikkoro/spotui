package cover

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCacheServesFromDiskAndSharesDownloads(t *testing.T) {
	data := pngBytes(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := NewCache(t.TempDir(), 0)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), srv.URL+"/a"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d downloads for concurrent requests, want 1", n)
	}

	srv.Close() // offline now: must come from disk
	if _, err := c.Get(context.Background(), srv.URL+"/a"); err != nil {
		t.Fatalf("cached cover not served offline: %v", err)
	}
}

func TestCachePrunesOldest(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 100)
	old := time.Now().Add(-time.Hour)
	for i, name := range []string{"a", "b", "c"} {
		p := dir + "/" + name
		_ = os.WriteFile(p, make([]byte, 40), 0o600)
		ts := old.Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(p, ts, ts)
	}
	c.Prune()
	if _, err := os.Stat(dir + "/a"); err == nil {
		t.Error("oldest entry should be gone")
	}
	if _, err := os.Stat(dir + "/c"); err != nil {
		t.Error("newest entry should stay")
	}
}
