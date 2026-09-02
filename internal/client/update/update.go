// Package update downloads a newer linux board-client from a GitHub Release.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	defaultTimeout = 45 * time.Second
	defaultAPIBase = "https://api.github.com"
)

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

	assetURLs map[string]string
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
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
		Client:   &http.Client{Timeout: timeout},
		ExecPath: os.Executable,
		Exec:     syscall.Exec,
	}
}

func (u *Updater) apiBase() string {
	if u.APIBase != "" {
		return strings.TrimRight(u.APIBase, "/")
	}
	return defaultAPIBase
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
	rawURL := bin.DownloadURL
	if rawURL == "" {
		rawURL = u.fileURL(bin.Name)
	}
	data, err := u.download(ctx, rawURL, bin.Size, true)
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

func (u *Updater) fileURL(name string) string {
	if u.assetURLs != nil {
		if s := u.assetURLs[name]; s != "" {
			return s
		}
	}
	return u.BaseURL + "/" + name
}

func (u *Updater) fetchManifest(ctx context.Context) (Manifest, error) {
	var zero Manifest
	if apiURL, ok := githubReleaseAPI(u.BaseURL, u.apiBase()); ok {
		if err := u.loadGitHubAssets(ctx, apiURL); err != nil {
			return zero, err
		}
	}
	body, err := u.download(ctx, u.fileURL("manifest.json"), 1<<20, true)
	if err != nil {
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
	for _, a := range rel.Assets {
		if a.Name != "" && a.URL != "" {
			urls[a.Name] = a.URL
		}
	}
	if urls["manifest.json"] == "" {
		return fmt.Errorf("github release has no manifest.json asset")
	}
	u.assetURLs = urls
	return nil
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
