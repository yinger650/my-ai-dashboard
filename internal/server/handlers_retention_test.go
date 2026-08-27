package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

func TestSettingsExposeMonthAndFiveGigQuota(t *testing.T) {
	srv, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/admin/settings", "", nil)
	if code != 200 {
		t.Fatalf("settings %d %s", code, env.Data)
	}
	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["event_retention_days"] != float64(30) {
		t.Fatalf("event days %+v", got["event_retention_days"])
	}
	if got["access_retention_days"] != float64(30) {
		t.Fatalf("access days %+v", got["access_retention_days"])
	}
	if got["event_quota_bytes"] != float64(5*1024*1024*1024) {
		t.Fatalf("quota %+v", got["event_quota_bytes"])
	}
}

func TestMaintenanceRunOK(t *testing.T) {
	srv, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	if code != 200 {
		t.Fatalf("session %d", code)
	}
	var sess struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(env.Data, &sess)
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/maintenance/run", sess.CSRFToken, map[string]any{})
	if code != 200 {
		t.Fatalf("maintenance %d %s", code, env.Data)
	}
	if !json.Valid(env.Data) {
		t.Fatalf("invalid json %s", env.Data)
	}
}
