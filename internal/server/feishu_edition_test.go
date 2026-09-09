package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/config"
	"agentboard/internal/event"
	"agentboard/internal/feishu"
	"agentboard/internal/shared"
	"agentboard/internal/workspace"
)

func newFeishuTestServer(t *testing.T, tenantKey string) (*httptest.Server, *workspace.Hub) {
	t.Helper()
	dir := t.TempDir()
	ctrl, err := workspace.OpenControl(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	hub := workspace.NewHub(dir, ctrl, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = hub.Close() })

	mux := http.NewServeMux()
	mux.HandleFunc("/open-apis/authen/v2/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "good-code" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 999, "msg": "bad code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "access_token": "user-tok"})
	})
	mux.HandleFunc("/open-apis/authen/v1/user_info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-tok" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 999, "msg": "bad token"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"open_id": "ou_test", "union_id": "on_test", "tenant_key": "tenantA",
			"name": "测试用户", "avatar_url": "",
		}})
	})
	oauthSrv := httptest.NewServer(mux)
	t.Cleanup(oauthSrv.Close)

	cfg := &config.Config{
		ListenAddr:         "127.0.0.1:0",
		PublicURL:          "http://127.0.0.1",
		DataDir:            dir,
		SessionHours:       12,
		SecureCookies:      false,
		FeishuAppID:        "cli_test",
		FeishuAppSecret:    "secret",
		FeishuRedirectURI:  "http://127.0.0.1/auth/feishu/callback",
		FeishuTenantKey:    tenantKey,
		FeishuAPIBase:      oauthSrv.URL,
		FeishuAuthorizeURL: oauthSrv.URL + "/authorize",
		MaxUploadBytes:     1024 * 1024,
		ArtifactQuotaBytes: 10 * 1024 * 1024,
	}
	oauth := &feishu.OAuth{
		AppID: cfg.FeishuAppID, AppSecret: cfg.FeishuAppSecret, RedirectURI: cfg.FeishuRedirectURI,
		APIBase: cfg.FeishuAPIBase, AuthorizeURL: cfg.FeishuAuthorizeURL, HTTPClient: oauthSrv.Client(),
	}
	s := NewFeishu(cfg, hub, oauth, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, make([]byte, 32))
	srv := httptest.NewServer(s.Router())
	t.Cleanup(srv.Close)
	return srv, hub
}

func TestFeishuAuthMetaAndPasswordLoginGone(t *testing.T) {
	srv, _ := newFeishuTestServer(t, "")
	res, err := http.Get(srv.URL + "/auth/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var env envelope
	_ = json.NewDecoder(res.Body).Decode(&env)
	var data map[string]any
	_ = json.Unmarshal(env.Data, &data)
	if data["edition"] != "feishu" {
		t.Fatalf("edition=%v", data["edition"])
	}
	code, _ := doJSON(t, http.DefaultClient, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "x"})
	if code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("password login should be absent, got %d", code)
	}
}

func TestFeishuOAuthIsolatesIngest(t *testing.T) {
	srv, hub := newFeishuTestServer(t, "")
	if err := hub.Control().PutOAuthState(t.Context(), "st1", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	noredirect := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := noredirect.Get(srv.URL + "/auth/feishu/callback?code=good-code&state=st1")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback status %d", res.StatusCode)
	}
	u, _ := url.Parse(srv.URL)
	if len(jar.Cookies(u)) == 0 {
		t.Fatal("expected session cookie")
	}

	client := &http.Client{Jar: jar}
	code, env := doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	if code != 200 {
		t.Fatalf("session %d", code)
	}
	var sess map[string]any
	_ = json.Unmarshal(env.Data, &sess)
	if sess["authenticated"] != true {
		t.Fatalf("sess=%v", sess)
	}
	slug, _ := sess["workspace_slug"].(string)
	if !workspace.ValidSlug(slug) {
		t.Fatalf("slug %q", slug)
	}

	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/board", "", nil)
	if code != 200 {
		t.Fatalf("board %d %s", code, env.Data)
	}

	csrf, _ := sess["csrf_token"].(string)
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/machines", csrf, map[string]any{
		"machine_key": "dev", "name": "Dev", "kind": "virtual", "create_machine_token": true,
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create machine %d %s", code, env.Data)
	}
	var created struct {
		Token *struct {
			Token string `json:"token"`
		} `json:"token"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if created.Token == nil || created.Token.Token == "" {
		t.Fatalf("no machine token in %s", env.Data)
	}

	rootPing, err := http.NewRequest(http.MethodGet, srv.URL+"/ingest/v1/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	rootPing.Header.Set("Authorization", "Bearer "+created.Token.Token)
	resp, err := http.DefaultClient.Do(rootPing)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("root ingest should 404 on feishu edition, got %d", resp.StatusCode)
	}

	payload, _ := json.Marshal(event.Heartbeat{Hostname: "h", OS: "linux", Arch: "amd64", HeartbeatIntervalSeconds: 30})
	ev := event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		EventType:     event.TypeHeartbeat,
		Payload:       payload,
	}
	body, _ := json.Marshal(map[string]any{"events": []event.Envelope{ev}})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/"+slug+"/ingest/v1/events", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+created.Token.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ingest %d %s", resp.StatusCode, raw)
	}

	wrong := strings.Repeat("a", 8)
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/"+wrong+"/ingest/v1/events", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+created.Token.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown slug ingest got %d", resp.StatusCode)
	}
}

func TestPersonalAuthMeta(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := http.Get(srv.URL + "/auth/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var env envelope
	_ = json.NewDecoder(res.Body).Decode(&env)
	var data map[string]any
	_ = json.Unmarshal(env.Data, &data)
	if data["edition"] != "personal" {
		t.Fatalf("edition=%v", data["edition"])
	}
}
