package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.1.6", "v0.1.5", true},
		{"v0.2.0", "v0.1.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.1.5", "v0.1.5", false},
		{"v0.1.4", "v0.1.5", false},
		{"v0.1.5", "dev", true},
		{"dev", "v0.1.5", false},
		{"v0.1.10", "v0.1.9", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func rel(tag string, pre bool) Release {
	return Release{Tag: tag, Prerelease: pre, Assets: []Asset{{Name: AssetName(), URL: "https://x/" + tag}}}
}

func TestPick(t *testing.T) {
	if AssetName() == "" {
		t.Skip("no published binary for this platform")
	}
	rels := []Release{rel("v0.1.6", true), rel("v0.1.4", false), rel("v0.1.5", true), {Tag: "v0.2.0", Draft: true}}
	if r := Pick(rels, false); r == nil || r.Tag != "v0.1.4" {
		t.Fatalf("stable pick: %+v", r)
	}
	if r := Pick(rels, true); r == nil || r.Tag != "v0.1.6" {
		t.Fatalf("pre-release pick: %+v", r)
	}
	if r := Pick([]Release{rel("v0.1.6", true)}, false); r != nil {
		t.Fatalf("only pre-releases and pre off: %+v", r)
	}
}

func TestReleasesCachesAndBacksOff(t *testing.T) {
	calls := 0
	limited := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if limited {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("If-None-Match") == `"abc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		_ = json.NewEncoder(w).Encode([]Release{rel("v0.1.6", true)})
	}))
	defer srv.Close()

	now := time.Unix(1_700_000_000, 0)
	c := NewChecker(t.TempDir(), "v0.1.5")
	c.API, c.Client, c.Now = srv.URL, srv.Client(), func() time.Time { return now }
	ctx := context.Background()

	if rels, err := c.Releases(ctx, false); err != nil || len(rels) != 1 {
		t.Fatalf("first: %v %v", rels, err)
	}
	// Within a day: served from the cache, GitHub not asked.
	now = now.Add(time.Hour)
	if _, err := c.Releases(ctx, false); err != nil || calls != 1 {
		t.Fatalf("cached: calls %d, %v", calls, err)
	}
	// Forced: asked with the ETag, a 304 keeps the cached list.
	if rels, err := c.Releases(ctx, true); err != nil || len(rels) != 1 || calls != 2 {
		t.Fatalf("304: calls %d, %v %v", calls, rels, err)
	}
	// Rate limited: remembered, and nothing is sent until it resets.
	limited = true
	var rl *ErrRateLimited
	if _, err := c.Releases(ctx, true); !errors.As(err, &rl) {
		t.Fatalf("want rate limit, got %v", err)
	}
	if _, err := c.Releases(ctx, true); !errors.As(err, &rl) || calls != 3 {
		t.Fatalf("asked again while limited: calls %d", calls)
	}
	// The cached list still answers a non-forced check meanwhile.
	if rels, err := c.Releases(ctx, false); err != nil || len(rels) != 1 {
		t.Fatalf("cached during limit: %v %v", rels, err)
	}
}

func tarGz(t *testing.T, name string, data []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hi"))
	_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(data)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	got, err := extract(tarGz(t, "spotui", []byte("BIN")), "spotui-linux-amd64.tar.gz", "spotui")
	if err != nil || string(got) != "BIN" {
		t.Fatalf("tar: %q %v", got, err)
	}

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	f, _ := zw.Create("spotui.exe")
	_, _ = f.Write([]byte("EXE"))
	_ = zw.Close()
	got, err = extract(zbuf.Bytes(), "spotui-windows-amd64.zip", "spotui.exe")
	if err != nil || string(got) != "EXE" {
		t.Fatalf("zip: %q %v", got, err)
	}
}

func TestVerify(t *testing.T) {
	data := []byte("archive")
	sum := sha256.Sum256(data)
	list := []byte(hex.EncodeToString(sum[:]) + "  spotui-linux-amd64.tar.gz\nabc  other.zip\n")
	if err := verify(data, list, "spotui-linux-amd64.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := verify([]byte("tampered"), list, "spotui-linux-amd64.tar.gz"); err == nil {
		t.Fatal("a wrong checksum must fail")
	}
	if err := verify(data, list, "missing.tar.gz"); err == nil {
		t.Fatal("an unlisted archive must fail")
	}
}

func TestInstallReplacesBinary(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("archive layout tested on linux/amd64")
	}
	archive := tarGz(t, "spotui", []byte("NEW"))
	sum := sha256.Sum256(archive)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(archive)
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  spotui-linux-amd64.tar.gz\n"))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "spotui")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Release{Tag: "v9.9.9", Assets: []Asset{
		{Name: "spotui-linux-amd64.tar.gz", URL: srv.URL + "/spotui-linux-amd64.tar.gz"},
		{Name: "SHA256SUMS", URL: srv.URL + "/SHA256SUMS"},
	}}
	if err := Install(context.Background(), srv.Client(), r, exe, "v0.1.5"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Fatalf("binary is %q", b)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm()&0o100 == 0 {
		t.Fatal("not executable")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".spotui-update-*")); len(left) > 0 {
		t.Fatalf("left behind %v", left)
	}
}
