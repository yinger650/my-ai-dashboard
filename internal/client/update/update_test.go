package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSameCommit(t *testing.T) {
	if !SameCommit("c762702", "c7627027612fea94") {
		t.Fatal("short vs full")
	}
	if SameCommit("dev", "abc123") {
		t.Fatal("dev should not match")
	}
	if SameCommit("aaaaaaaa", "bbbbbbbb") {
		t.Fatal("different commits")
	}
}

func TestCheckAndApply(t *testing.T) {
	payload := []byte("fake-linux-client-binary")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	manifest := `{
  "schema": 1,
  "name": "board-client",
  "version": "0.1.10",
  "commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "binaries": {
    "linux-amd64": {"name": "board-client-linux-amd64", "sha256": "` + digest + `", "size": ` + strconv.Itoa(len(payload)) + `}
  }
}`
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/board-client-linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	exe := filepath.Join(dir, "board-client")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := New(srv.URL, Info{Version: "0.1.9", Commit: "bbbbbbbbbbbbbbbb"}, 0)
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.ExecPath = func() (string, error) { return exe, nil }
	execed := false
	u.Exec = func(argv0 string, argv []string, envv []string) error {
		execed = true
		if argv0 != exe {
			t.Errorf("exec path %q", argv0)
		}
		return nil
	}

	man, bin, ok, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected update, commit %s", man.Commit)
	}
	if err := u.Apply(context.Background(), bin); err != nil {
		t.Fatal(err)
	}
	if !execed {
		t.Fatal("expected exec after replace")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("replaced bytes = %q", got)
	}

	u.Current.Commit = man.Commit
	_, _, ok, err = u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("same commit should not need update")
	}
}

func TestRejectsBadHash(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"commit":"deadbeef","binaries":{"linux-amd64":{"name":"board-client-linux-amd64","sha256":"00","size":4}}}`))
	})
	mux.HandleFunc("/board-client-linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("nope"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	exe := filepath.Join(dir, "board-client")
	_ = os.WriteFile(exe, []byte("old"), 0o755)
	u := New(srv.URL, Info{Commit: "1111"}, 0)
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.ExecPath = func() (string, error) { return exe, nil }
	u.Exec = func(string, []string, []string) error { t.Fatal("must not exec"); return nil }
	_, bin, ok, err := u.Check(context.Background())
	if err != nil || !ok {
		t.Fatalf("check %v ok=%v", err, ok)
	}
	if err := u.Apply(context.Background(), bin); err == nil {
		t.Fatal("expected sha mismatch")
	}
}

func TestGitHubReleaseAPI(t *testing.T) {
	api := "https://api.github.com"
	got, ok := githubReleaseAPI("https://github.com/yinger650/my-ai-dashboard/releases/latest/download", api)
	if !ok || got != "https://api.github.com/repos/yinger650/my-ai-dashboard/releases/latest" {
		t.Fatalf("latest download -> %q ok=%v", got, ok)
	}
	got, ok = githubReleaseAPI("https://github.com/yinger650/my-ai-dashboard/releases/download/board-client", api)
	if !ok || got != "https://api.github.com/repos/yinger650/my-ai-dashboard/releases/tags/board-client" {
		t.Fatalf("tag download -> %q ok=%v", got, ok)
	}
	if _, ok := githubReleaseAPI("http://127.0.0.1:9/manifest.json", api); ok {
		t.Fatal("direct URL should not look like GitHub")
	}
}

func TestSourcesPrefersBoardMirror(t *testing.T) {
	got := Sources("http://127.0.0.1:8090", "https://github.com/yinger650/my-ai-dashboard/releases/latest/download")
	if len(got) != 2 {
		t.Fatalf("sources = %v", got)
	}
	if got[0] != "http://127.0.0.1:8090/client-updates" {
		t.Fatalf("first source %q", got[0])
	}
	if got[1] != "https://github.com/yinger650/my-ai-dashboard/releases/latest/download" {
		t.Fatalf("second source %q", got[1])
	}
	dup := Sources("https://board.yinger650.com/", "https://board.yinger650.com/client-updates")
	if len(dup) != 1 || dup[0] != "https://board.yinger650.com/client-updates" {
		t.Fatalf("dedupe = %v", dup)
	}
}

func TestGitHubAPIFallbackToDirectURL(t *testing.T) {
	payload := []byte("fallback-linux-client")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	manifest := `{"commit":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","binaries":{"linux-amd64":{"name":"board-client-linux-amd64","sha256":"` + digest + `","size":` + strconv.Itoa(len(payload)) + `}}}`

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/yinger650/my-ai-dashboard/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "api down", http.StatusBadGateway)
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/board-client-linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "board-client")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := New("https://github.com/yinger650/my-ai-dashboard/releases/latest/download", Info{Commit: "ffff"}, 0)
	u.APIBase = srv.URL
	u.Client.Transport = rewriteHost{host: "github.com", target: target, next: u.Client.Transport}
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.ExecPath = func() (string, error) { return exe, nil }
	u.Exec = func(string, []string, []string) error { return nil }

	_, bin, ok, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("api failure should fall back to direct URL: %v", err)
	}
	if !ok {
		t.Fatal("expected update via direct URL fallback")
	}
	if err := u.Apply(context.Background(), bin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}

type rewriteHost struct {
	host   string
	target *url.URL
	next   http.RoundTripper
}

func (t rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	if strings.EqualFold(r.URL.Host, t.host) {
		u := *t.target
		u.Path = strings.TrimRight(t.target.Path, "/") + "/" + path.Base(r.URL.Path)
		r.URL = &u
		r.Host = u.Host
	}
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(r)
}

func TestGitHubAPIThenCDN(t *testing.T) {
	payload := []byte("cdn-linux-client")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	manifest := `{"commit":"cccccccccccccccccccccccccccccccccccccccc","binaries":{"linux-amd64":{"name":"board-client-linux-amd64","sha256":"` + digest + `","size":` + strconv.Itoa(len(payload)) + `}}}`

	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/yinger650/my-ai-dashboard/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") == "application/octet-stream" {
			t.Error("release json must not request octet-stream")
		}
		_, _ = w.Write([]byte(`{"tag_name":"board-client","assets":[
			{"name":"manifest.json","url":"` + base + `/assets/manifest","size":100},
			{"name":"board-client-linux-amd64","url":"` + base + `/assets/bin","size":16}
		]}`))
	})
	mux.HandleFunc("/assets/manifest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/octet-stream" {
			http.Error(w, "want octet-stream", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/cdn/manifest.json", http.StatusFound)
	})
	mux.HandleFunc("/assets/bin", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/octet-stream" {
			http.Error(w, "want octet-stream", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/cdn/board-client-linux-amd64", http.StatusFound)
	})
	mux.HandleFunc("/cdn/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/cdn/board-client-linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base = srv.URL

	dir := t.TempDir()
	exe := filepath.Join(dir, "board-client")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := New("https://github.com/yinger650/my-ai-dashboard/releases/latest/download", Info{Commit: "dddd"}, 0)
	u.APIBase = srv.URL
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.ExecPath = func() (string, error) { return exe, nil }
	u.Exec = func(string, []string, []string) error { return nil }

	_, bin, ok, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected update via GitHub API")
	}
	if !strings.Contains(bin.DownloadURL, "/assets/bin") {
		t.Fatalf("expected API asset URL, got %s", bin.DownloadURL)
	}
	if err := u.Apply(context.Background(), bin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}

func TestMirrorToWritesAssets(t *testing.T) {
	payload := []byte("mirror-linux-client")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	manifest := `{
  "schema": 1,
  "commit": "ffffffffffffffffffffffffffffffffffffffff",
  "binaries": {
    "linux-amd64": {"name": "board-client-linux-amd64", "sha256": "` + digest + `", "size": ` + strconv.Itoa(len(payload)) + `}
  }
}`
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/board-client-linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(digest + "  board-client-linux-amd64\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dest := t.TempDir()
	u := New(srv.URL, Info{Commit: "sync"}, 0)
	u.GOOS, u.GOARCH = "linux", "amd64"
	man, err := u.MirrorTo(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if man.Commit == "" {
		t.Fatal("empty commit")
	}
	got, err := os.ReadFile(filepath.Join(dest, "board-client-linux-amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}
