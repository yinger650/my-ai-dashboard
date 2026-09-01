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
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Info is the running binary's identity from ldflags.
type Info struct {
	Version string
	Commit  string
}

// Updater checks a release URL and optionally replaces the current executable.
type Updater struct {
	BaseURL  string
	Current  Info
	GOOS     string
	GOARCH   string
	Client   *http.Client
	ExecPath func() (string, error)
	Exec     func(argv0 string, argv []string, envv []string) error
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
	data, err := u.download(ctx, u.BaseURL+"/"+bin.Name, bin.Size)
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

func (u *Updater) fetchManifest(ctx context.Context) (Manifest, error) {
	var zero Manifest
	body, err := u.download(ctx, u.BaseURL+"/manifest.json", 1<<20)
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

func (u *Updater) download(ctx context.Context, rawURL string, hint int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AgentBoard-Client/"+strings.TrimSpace(u.Current.Version+" "+u.Current.Commit))
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
