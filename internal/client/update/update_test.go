package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
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
