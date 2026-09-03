package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"agentboard/internal/client/update"
)

func TestClientUpdatePutGetAndApply(t *testing.T) {
	srv, _ := newTestServer(t)
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

	put := func(name string, body []byte, token string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPut, srv.URL+"/client-updates/"+name, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}

	if put("manifest.json", []byte(manifest), "") != http.StatusForbidden {
		t.Fatal("expected forbidden without token")
	}
	if put("manifest.json", []byte(manifest), "update-secret") != http.StatusNoContent {
		t.Fatal("put manifest")
	}
	if put("board-client-linux-amd64", payload, "update-secret") != http.StatusNoContent {
		t.Fatal("put binary")
	}
	if put("../etc/passwd", []byte("x"), "update-secret") != http.StatusNotFound {
		t.Fatal("expected traversal to 404")
	}

	got, err := http.Get(srv.URL + "/client-updates/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if got.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"commit"`)) {
		t.Fatalf("get manifest %d %s", got.StatusCode, body)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "board-client")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	up := update.New(srv.URL+"/client-updates", update.Info{Commit: "bbbb"}, 0)
	up.GOOS, up.GOARCH = "linux", "amd64"
	up.ExecPath = func() (string, error) { return exe, nil }
	up.Exec = func(string, []string, []string) error { return nil }
	_, bin, ok, err := up.Check(context.Background())
	if err != nil || !ok {
		t.Fatalf("check err=%v ok=%v", err, ok)
	}
	if err := up.Apply(context.Background(), bin); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(payload) {
		t.Fatalf("replaced bytes = %q", out)
	}
}

func TestClientUpdateRejectsUnknownName(t *testing.T) {
	srv, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/client-updates/evil.bin", bytes.NewReader([]byte("x")))
	req.Header.Set("Authorization", "Bearer update-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
