// Package update finds newer spotui releases on GitHub and installs them in
// place of the running binary.
//
// GitHub allows 60 unauthenticated API requests an hour per address. A check
// is one request, sent with the ETag of the last answer (a 304 does not count
// against the limit); the interface checks at most once a day, and a rate
// limit answer is remembered until GitHub says it resets.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are published.
const Repo = "Nikkoro/spotui"

// Release is the part of GitHub's release object used here.
type Release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	URL        string  `json:"html_url"`
	Assets     []Asset `json:"assets"`
}

// Asset is a file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// ErrRateLimited reports that GitHub asked to wait; Until says how long.
type ErrRateLimited struct{ Until time.Time }

func (e *ErrRateLimited) Error() string {
	return "GitHub rate limit reached, try again after " + e.Until.Local().Format("15:04")
}

// Checker asks GitHub for releases, caching the answer in Dir.
type Checker struct {
	Dir        string // where update.json is kept
	Version    string // the running version, for the User-Agent
	Client     *http.Client
	API        string // https://api.github.com; tests point it elsewhere
	Now        func() time.Time
	CheckEvery time.Duration // how old a cached answer may be for Check
}

// NewChecker returns a Checker with GitHub's API and a day between checks.
func NewChecker(dir, version string) *Checker {
	return &Checker{
		Dir: dir, Version: version,
		Client:     &http.Client{Timeout: 30 * time.Second},
		API:        "https://api.github.com",
		Now:        time.Now,
		CheckEvery: 24 * time.Hour,
	}
}

type cacheFile struct {
	CheckedAt time.Time       `json:"checked_at"`
	ETag      string          `json:"etag"`
	NotBefore time.Time       `json:"not_before"`
	Releases  json.RawMessage `json:"releases"`
}

func (c *Checker) cachePath() string { return filepath.Join(c.Dir, "update.json") }

func (c *Checker) readCache() cacheFile {
	var f cacheFile
	if b, err := os.ReadFile(c.cachePath()); err == nil {
		_ = json.Unmarshal(b, &f)
	}
	return f
}

func (c *Checker) writeCache(f cacheFile) {
	if b, err := json.Marshal(f); err == nil {
		_ = os.MkdirAll(c.Dir, 0o700)
		_ = os.WriteFile(c.cachePath(), b, 0o600)
	}
}

// Releases returns the published releases, newest first. With fresh set it
// asks GitHub (conditionally); otherwise a cached answer younger than
// CheckEvery is used as is.
func (c *Checker) Releases(ctx context.Context, fresh bool) ([]Release, error) {
	cache := c.readCache()
	now := c.Now()
	if now.Before(cache.NotBefore) {
		if len(cache.Releases) > 0 && !fresh {
			return decode(cache.Releases)
		}
		return nil, &ErrRateLimited{Until: cache.NotBefore}
	}
	if !fresh && len(cache.Releases) > 0 && now.Sub(cache.CheckedAt) < c.CheckEvery {
		return decode(cache.Releases)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+"/repos/"+Repo+"/releases?per_page=20", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "spotui/"+c.Version+" (+https://github.com/"+Repo+")")
	if cache.ETag != "" && len(cache.Releases) > 0 {
		req.Header.Set("If-None-Match", cache.ETag)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking GitHub for releases: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		cache.CheckedAt = now
		c.writeCache(cache)
		return decode(cache.Releases)
	case resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"):
		cache.NotBefore = retryAt(resp.Header, now)
		c.writeCache(cache)
		return nil, &ErrRateLimited{Until: cache.NotBefore}
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	rels, err := decode(body)
	if err != nil {
		return nil, err
	}
	c.writeCache(cacheFile{CheckedAt: now, ETag: resp.Header.Get("ETag"), Releases: body})
	return rels, nil
}

// retryAt reads when GitHub allows the next request: Retry-After, else the
// rate limit reset, else an hour.
func retryAt(h http.Header, now time.Time) time.Time {
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil && s > 0 {
		return now.Add(time.Duration(s) * time.Second)
	}
	if s, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil && s > now.Unix() {
		return time.Unix(s, 0)
	}
	return now.Add(time.Hour)
}

func decode(b []byte) ([]Release, error) {
	var rels []Release
	if err := json.Unmarshal(b, &rels); err != nil {
		return nil, fmt.Errorf("reading GitHub's answer: %w", err)
	}
	return rels, nil
}

// Pick returns the newest release with this platform's binary: stable ones
// only unless pre is set. Drafts never count.
func Pick(rels []Release, pre bool) *Release {
	var best *Release
	for i := range rels {
		r := &rels[i]
		if r.Draft || (r.Prerelease && !pre) || r.asset(AssetName()) == nil {
			continue
		}
		if best == nil || Newer(r.Tag, best.Tag) {
			best = r
		}
	}
	return best
}

func (r *Release) asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Newer reports whether version a is newer than b ("v1.2.3"). A version
// that does not parse, such as a development build, is never newer, and
// anything is newer than it.
func Newer(a, b string) bool {
	va, okA := parse(a)
	vb, okB := parse(b)
	switch {
	case !okA:
		return false
	case !okB:
		return true
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AssetName is the archive for this platform, or "" if none is published.
func AssetName() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "spotui-linux-amd64.tar.gz"
	case "windows/amd64":
		return "spotui-windows-amd64.zip"
	}
	return ""
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "spotui.exe"
	}
	return "spotui"
}

// maxArchive bounds downloads; a release archive is a few tens of MB.
const maxArchive = 200 << 20

// Install downloads rel's archive for this platform, checks it against the
// release's SHA256SUMS when it has one, and replaces the binary at exe.
func Install(ctx context.Context, client *http.Client, rel *Release, exe string, version string) error {
	name := AssetName()
	a := rel.asset(name)
	if a == nil {
		return fmt.Errorf("%s has no %s", rel.Tag, name)
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}

	archive, err := download(ctx, client, a.URL, version, maxArchive)
	if err != nil {
		return err
	}
	if sums := rel.asset("SHA256SUMS"); sums != nil {
		list, err := download(ctx, client, sums.URL, version, 1<<20)
		if err != nil {
			return fmt.Errorf("checksums: %w", err)
		}
		if err := verify(archive, list, name); err != nil {
			return err
		}
	}

	bin, err := extract(archive, name, binaryName())
	if err != nil {
		return err
	}
	return replace(exe, bin)
}

func download(ctx context.Context, client *http.Client, url, version string, limit int64) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("refusing to download over plain http: %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "spotui/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", url)
	}
	return b, nil
}

// verify checks data against name's line in a sha256sum listing.
func verify(data, list []byte, name string) error {
	sc := bufio.NewScanner(strings.NewReader(string(list)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			sum := sha256.Sum256(data)
			if !strings.EqualFold(fields[0], hex.EncodeToString(sum[:])) {
				return fmt.Errorf("%s does not match its checksum; not installed", name)
			}
			return nil
		}
	}
	return fmt.Errorf("SHA256SUMS lists no %s; not installed", name)
}

// extract returns the file called bin from a .tar.gz or .zip archive.
func extract(archive []byte, archiveName, bin string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		zr, err := zip.NewReader(strings.NewReader(string(archive)), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", archiveName, err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == bin && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, maxArchive))
			}
		}
		return nil, fmt.Errorf("%s has no %s", archiveName, bin)
	}

	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", archiveName, err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s has no %s", archiveName, bin)
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", archiveName, err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == bin {
			return io.ReadAll(io.LimitReader(tr, maxArchive))
		}
	}
}

// replace swaps the binary at exe for bin. The new file is written next to
// it and renamed over it, so a failure leaves the old binary in place. On
// Windows a running .exe cannot be overwritten but can be renamed: it is
// moved aside to .old, which CleanupOld removes on the next start.
func replace(exe string, bin []byte) error {
	if len(bin) == 0 {
		return errors.New("the new binary is empty")
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".spotui-update-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s (%w); is it in a folder you can write to?", exe, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // gone already once renamed

	mode := os.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm() | 0o100
	}
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("moving the running binary aside: %w", err)
		}
		if err := os.Rename(tmpName, exe); err != nil {
			_ = os.Rename(old, exe)
			return fmt.Errorf("installing: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpName, exe); err != nil {
		return fmt.Errorf("installing: %w", err)
	}
	return nil
}

// CleanupOld removes the binary a Windows update moved aside.
func CleanupOld(exe string) {
	if runtime.GOOS == "windows" {
		_ = os.Remove(exe + ".old")
	}
}

// Executable is the running binary's real path, symlinks resolved.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
