// Package update downloads a newer linux board-client from a GitHub Release
// or from a board-server /client-updates mirror.
package update

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	// MaxBinaryBytes rejects oversized downloads.
	MaxBinaryBytes = 80 << 20
	// MirrorPath is the board-server route that hosts rolling client assets.
	MirrorPath     = "/client-updates"
	defaultTimeout = 10 * time.Minute
	defaultAPIBase = "https://api.github.com"
)

// AllowedNames is the set of files a mirror may host or a client may fetch.
var AllowedNames = map[string]bool{
	"manifest.json":            true,
	"SHA256SUMS":               true,
	"board-client-linux-amd64": true,
	"board-client-linux-arm64": true,
}

// Manifest describes the rolling board-client release.
type Manifest struct {
	Schema    int               `json:"schema"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Commit    string            `json:"commit"`
	BuildTime string            `json:"build_time"`
	Binaries  map[string]Binary `json:"binaries"`
}

// Binary is one platform asset in the manifest.
type Binary struct {
	Name        string `json:"name"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"-"`
}

// Info is the running binary's identity from ldflags.
type Info struct {
	Version string
	Commit  string
}

// Updater checks a release URL and optionally replaces the current executable.
type Updater struct {
	BaseURL  string
	APIBase  string
	Current  Info
	GOOS     string
	GOARCH   string
	Client   *http.Client
	ExecPath func() (string, error)
	Exec     func(argv0 string, argv []string, envv []string) error

	assetURLs   map[string]string
	browserURLs map[string]string
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// New returns an updater for the running process.
func New(baseURL string, current Info, timeout time.Duration) *Updater {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Updater{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Current:  current,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Client:   newHTTPClient(timeout),
		ExecPath: os.Executable,
		Exec:     syscall.Exec,
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := dialer.DialContext(ctx, "tcp4", addr)
			if err == nil {
				return c, nil
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(authority string, c *tls.Conn) http.RoundTripper{},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if len(via) > 0 && req.URL.Host != via[len(via)-1].URL.Host {
				req.Header.Del("Accept")
			}
			return nil
		},
	}
}

func (u *Updater) apiBase() string {
	if u.APIBase != "" {
		return strings.TrimRight(u.APIBase, "/")
	}
	return defaultAPIBase
}

// BoardMirrorURL is the client-update route on a board-server base URL.
func BoardMirrorURL(serverURL string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/") + MirrorPath
}

// Sources lists upgrade endpoints, preferring the board-server mirror so
// machines that cannot reach GitHub (Tencent Cloud) still update.
func Sources(serverURL, configured string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			return
		}
		key := strings.ToLower(raw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, raw)
	}
	add(BoardMirrorURL(serverURL))
	add(configured)
	return out
}

// Platform is the manifest key, e.g. linux-amd64.
func Platform(goos, goarch string) string {
	return goos + "-" + goarch
}

// SameCommit reports whether two commit ids refer to the same revision.
func SameCommit(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" || a == "unknown" || a == "dev" || b == "unknown" || b == "dev" {
		return false
	}
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// githubReleaseAPI parses a github.com or api.github.com URL into the releases API path.
func githubReleaseAPI(raw, apiBase string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	apiBase = strings.TrimRight(apiBase, "/")
	if host == "api.github.com" {
		if len(parts) >= 4 && parts[0] == "repos" {
			return strings.TrimRight(raw, "/"), true
		}
		return "", false
	}
	if host != "github.com" && host != "www.github.com" {
		return "", false
	}
	if len(parts) < 2 {
		return "", false
	}
	owner, repo := parts[0], parts[1]
	tag := "latest"
	if len(parts) >= 4 && parts[2] == "releases" {
		switch parts[3] {
		case "latest":
			tag = "latest"
		case "download":
			if len(parts) >= 5 && parts[4] != "" {
				tag = parts[4]
			}
		default:
			tag = parts[3]
		}
	}
	if tag == "latest" {
		return apiBase + "/repos/" + owner + "/" + repo + "/releases/latest", true
	}
	return apiBase + "/repos/" + owner + "/" + repo + "/releases/tags/" + url.PathEscape(tag), true
}

// Check fetches the manifest. ok is false when this platform is already current.
func (u *Updater) Check(ctx context.Context) (man Manifest, bin Binary, ok bool, err error) {
	if u.BaseURL == "" {
		return man, bin, false, fmt.Errorf("update url is empty")
	}
	man, err = u.fetchManifest(ctx)
	if err != nil {
		return man, bin, false, err
	}
	key := Platform(u.GOOS, u.GOARCH)
	bin, found := man.Binaries[key]
	if !found || bin.Name == "" || bin.SHA256 == "" {
		return man, bin, false, fmt.Errorf("no board-client asset for %s", key)
	}
	bin.DownloadURL = u.fileURL(bin.Name)
	if SameCommit(u.Current.Commit, man.Commit) {
		return man, bin, false, nil
	}
	return man, bin, true, nil
}

// Apply downloads, verifies, replaces the executable, then execs it.
func (u *Updater) Apply(ctx context.Context, bin Binary) error {
	exe, err := u.ExecPath()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	data, err := u.downloadAsset(ctx, bin.Name, bin.Size)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := strings.ToLower(strings.TrimSpace(bin.SHA256))
	if got != want {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}
	if err := replaceExecutable(exe, data); err != nil {
		return err
	}
	if u.Exec == nil {
		return nil
	}
	return u.Exec(exe, os.Args, os.Environ())
}

// MirrorTo writes the rolling client assets into destDir (manifest + linux binaries).
func (u *Updater) MirrorTo(ctx context.Context, destDir string) (Manifest, error) {
	var zero Manifest
	man, err := u.fetchManifest(ctx)
	if err != nil {
		return zero, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return zero, err
	}
	body, err := u.downloadAsset(ctx, "manifest.json", 1<<20)
	if err != nil {
		return zero, err
	}
	if err := writeAtomic(filepath.Join(destDir, "manifest.json"), body, 0o644); err != nil {
		return zero, err
	}
	if sums, err := u.downloadAsset(ctx, "SHA256SUMS", 1<<20); err == nil {
		_ = writeAtomic(filepath.Join(destDir, "SHA256SUMS"), sums, 0o644)
	}
	for _, bin := range man.Binaries {
		if !AllowedNames[bin.Name] {
			continue
		}
		data, err := u.downloadAsset(ctx, bin.Name, bin.Size)
		if err != nil {
			return zero, fmt.Errorf("%s: %w", bin.Name, err)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		want := strings.ToLower(strings.TrimSpace(bin.SHA256))
		if got != want {
			return zero, fmt.Errorf("%s: sha256 mismatch", bin.Name)
		}
		if err := writeAtomic(filepath.Join(destDir, bin.Name), data, 0o755); err != nil {
			return zero, err
		}
	}
	return man, nil
}

func (u *Updater) fileURL(name string) string {
	if u.assetURLs != nil {
		if s := u.assetURLs[name]; s != "" {
			return s
		}
	}
	if u.browserURLs != nil {
		if s := u.browserURLs[name]; s != "" {
			return s
		}
	}
	return u.BaseURL + "/" + name
}

func (u *Updater) candidateURLs(name string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	if u.assetURLs != nil {
		add(u.assetURLs[name])
	}
	if u.browserURLs != nil {
		add(u.browserURLs[name])
	}
	add(u.BaseURL + "/" + name)
	return out
}

func (u *Updater) fetchManifest(ctx context.Context) (Manifest, error) {
	var zero Manifest
	var apiErr error
	if apiURL, ok := githubReleaseAPI(u.BaseURL, u.apiBase()); ok {
		if err := u.loadGitHubAssets(ctx, apiURL); err != nil {
			apiErr = err
			u.assetURLs = nil
			u.browserURLs = nil
		}
	}
	body, err := u.downloadAsset(ctx, "manifest.json", 1<<20)
	if err != nil {
		if apiErr != nil {
			return zero, fmt.Errorf("%w (github api: %v)", err, apiErr)
		}
		return zero, err
	}
	var man Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		return zero, fmt.Errorf("manifest: %w", err)
	}
	if man.Commit == "" || len(man.Binaries) == 0 {
		return zero, fmt.Errorf("manifest missing commit or binaries")
	}
	return man, nil
}

func (u *Updater) loadGitHubAssets(ctx context.Context, apiURL string) error {
	body, err := u.download(ctx, apiURL, 2<<20, false)
	if err != nil {
		return fmt.Errorf("github release: %w", err)
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return fmt.Errorf("github release json: %w", err)
	}
	urls := make(map[string]string, len(rel.Assets))
	browser := make(map[string]string, len(rel.Assets))
	for _, a := range rel.Assets {
		if a.Name == "" {
			continue
		}
		if a.URL != "" {
			urls[a.Name] = a.URL
		}
		if a.BrowserDownloadURL != "" {
			browser[a.Name] = a.BrowserDownloadURL
		}
	}
	if urls["manifest.json"] == "" && browser["manifest.json"] == "" {
		return fmt.Errorf("github release has no manifest.json asset")
	}
	u.assetURLs = urls
	u.browserURLs = browser
	return nil
}

func (u *Updater) downloadAsset(ctx context.Context, name string, hint int64) ([]byte, error) {
	if !AllowedNames[name] {
		return nil, fmt.Errorf("refusing to download %q", name)
	}
	var errs []error
	for _, raw := range u.candidateURLs(name) {
		data, err := u.download(ctx, raw, hint, true)
		if err == nil {
			return data, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("no download URL for %s", name)
	}
	return nil, errors.Join(errs...)
}

func (u *Updater) download(ctx context.Context, rawURL string, hint int64, octet bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AgentBoard-Client/"+strings.TrimSpace(u.Current.Version+" "+u.Current.Commit))
	if octet {
		req.Header.Set("Accept", "application/octet-stream")
	} else {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	limit := int64(MaxBinaryBytes)
	if hint > 0 && hint < limit {
		limit = hint + 4096
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeded %d bytes", limit)
	}
	return data, nil
}

func replaceExecutable(dest string, data []byte) error {
	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	mode := info.Mode()
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, data, mode.Perm()); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode.Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeAtomic(dest string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dest)+".")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	ok = true
	return nil
}
