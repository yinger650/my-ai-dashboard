package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/auth"
	"agentboard/internal/config"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("super-secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAdminPassword(t.Context(), hash); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{7}, 32)
	cfg := &config.Config{
		ListenAddr:         "127.0.0.1:0",
		PublicURL:          "http://127.0.0.1",
		DataDir:            dir,
		ArtifactDir:        filepath.Join(dir, "artifacts"),
		MaxUploadBytes:     1024 * 1024,
		ArtifactQuotaBytes: 10 * 1024 * 1024,
		SessionHours:       12,
		SecureCookies:      false,
	}
	if err := os.MkdirAll(cfg.ArtifactDir, 0o750); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, key)
	srv := httptest.NewServer(s.Router())
	t.Cleanup(func() {
		srv.Close()
		_ = st.Close()
	})
	return srv, st
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doJSON(t *testing.T, client *http.Client, method, rawURL, csrf string, body any) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	_ = json.Unmarshal(raw, &env)
	return resp.StatusCode, env
}

func TestTOTPAndArtifactAndSummarize(t *testing.T) {
	srv, st := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d %+v", code, env)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	if code != 200 {
		t.Fatalf("session %d", code)
	}
	var sess struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(env.Data, &sess)
	if sess.CSRFToken == "" {
		t.Fatal("missing csrf")
	}

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/totp/setup", sess.CSRFToken, map[string]any{})
	if code != 200 {
		t.Fatalf("setup %d %+v", code, env)
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(env.Data, &setup)
	totp := auth.CurrentTOTP(setup.Secret, time.Now())
	if totp == "" {
		t.Fatal("empty totp")
	}

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/totp/confirm", sess.CSRFToken, map[string]string{"code": totp})
	if code != 200 {
		t.Fatalf("confirm %d %+v", code, env)
	}
	var confirmed struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	_ = json.Unmarshal(env.Data, &confirmed)
	if len(confirmed.RecoveryCodes) != 10 {
		t.Fatalf("recovery = %d", len(confirmed.RecoveryCodes))
	}

	_, _ = doJSON(t, client, http.MethodPost, srv.URL+"/auth/logout", sess.CSRFToken, map[string]any{})
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 401 || env.Error == nil || env.Error.Code != "totp_required" {
		t.Fatalf("expected totp_required, got %d %+v", code, env)
	}
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{
		"password":  "super-secret-password",
		"totp_code": auth.CurrentTOTP(setup.Secret, time.Now()),
	})
	if code != 200 {
		t.Fatalf("totp login %d %+v", code, env)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	_ = json.Unmarshal(env.Data, &sess)

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/machines", sess.CSRFToken, map[string]any{
		"machine_key": "testbox", "name": "Test", "kind": "vm",
	})
	if code != 201 {
		t.Fatalf("machine %d %+v", code, env)
	}
	var wrap struct {
		Machine struct {
			ID string `json:"id"`
		} `json:"machine"`
	}
	_ = json.Unmarshal(env.Data, &wrap)
	if wrap.Machine.ID == "" {
		t.Fatalf("no machine id: %s", env.Data)
	}

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/services", sess.CSRFToken, map[string]any{
		"machine_id": wrap.Machine.ID, "service_key": "demo", "name": "Demo", "type": "daemon",
	})
	if code != 201 {
		t.Fatalf("service %d %+v", code, env)
	}
	var svc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &svc)
	if svc.ID == "" {
		t.Fatalf("no service id: %s", env.Data)
	}

	now := shared.FormatTime(shared.NowUTC())
	pb, _ := json.Marshal(event.LogPayload{Markdown: "ERROR: boom failed", Severity: "error", Source: "t"})
	_, err := st.IngestEvent(t.Context(), &event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeLogAppend,
		OccurredAt:    now,
		ServiceKey:    "demo",
		Payload:       pb,
	}, store.IngestAuth{MachineID: wrap.Machine.ID, ServiceID: &svc.ID}, now)
	if err != nil {
		t.Fatal(err)
	}

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/services/"+svc.ID+"/summarize", sess.CSRFToken, map[string]any{"pin": true})
	if code != 200 {
		t.Fatalf("summarize %d %+v", code, env)
	}
	if !strings.Contains(string(env.Data), "boom") && !strings.Contains(string(env.Data), "日志") {
		t.Fatalf("summary missing content: %s", env.Data)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "note.txt")
	_, _ = fw.Write([]byte("hello artifact"))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/services/"+svc.ID+"/artifacts", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", sess.CSRFToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload %d %s", resp.StatusCode, raw)
	}

	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/services/"+svc.ID+"/artifacts", "", nil)
	if code != 200 {
		t.Fatalf("list artifacts %d %s", code, env.Data)
	}
	if !strings.Contains(string(env.Data), "note.txt") {
		t.Fatalf("artifact list: %s", env.Data)
	}
}
