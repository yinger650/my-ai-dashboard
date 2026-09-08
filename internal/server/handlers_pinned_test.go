package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func TestDeletePinnedLog(t *testing.T) {
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

	m := &store.Machine{MachineKey: "pinbox", Name: "Pin", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	auth := store.IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.FormatTime(shared.NowUTC())
	if r, err := st.IngestEvent(t.Context(), mkServerEnv(t, event.TypeServiceState, "app", event.ServiceState{
		Name: "App", Type: "daemon", State: "running", Severity: "normal",
	}), auth, now); err != nil || r.Status != "accepted" {
		t.Fatalf("state: %v %+v", err, r)
	}
	if r, err := st.IngestEvent(t.Context(), mkServerEnv(t, event.TypeLogPin, "app", event.LogPayload{
		Markdown: "keep me", Severity: "info",
	}), auth, now); err != nil || r.Status != "accepted" {
		t.Fatalf("pin: %v %+v", err, r)
	}
	svc, err := st.GetServiceByKey(t.Context(), m.ID, "app")
	if err != nil {
		t.Fatal(err)
	}

	code, env = doJSON(t, client, http.MethodDelete, srv.URL+"/api/v1/services/"+svc.ID+"/pinned", "", nil)
	if code != http.StatusForbidden && code != http.StatusUnauthorized {
		t.Fatalf("delete without csrf = %d %+v", code, env)
	}

	code, env = doJSON(t, client, http.MethodDelete, srv.URL+"/api/v1/services/"+svc.ID+"/pinned", sess.CSRFToken, nil)
	if code != 200 {
		t.Fatalf("delete pin %d %+v", code, env)
	}
	if _, err := st.GetPinnedLog(t.Context(), svc.ID); err != store.ErrNotFound {
		t.Fatalf("pin still in store: %v", err)
	}

	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/services/"+svc.ID, "", nil)
	if code != 200 {
		t.Fatalf("detail %d %+v", code, env)
	}
	var detail struct {
		Pinned *store.PinnedLog `json:"pinned"`
	}
	_ = json.Unmarshal(env.Data, &detail)
	if detail.Pinned != nil {
		t.Fatalf("detail still has pin: %+v", detail.Pinned)
	}

	code, env = doJSON(t, client, http.MethodDelete, srv.URL+"/api/v1/services/"+svc.ID+"/pinned", sess.CSRFToken, nil)
	if code != 404 {
		t.Fatalf("second delete %d %+v", code, env)
	}
}

func mkServerEnv(t *testing.T, etype, serviceKey string, payload any) *event.Envelope {
	t.Helper()
	pb, _ := json.Marshal(payload)
	return &event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     etype,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		ServiceKey:    serviceKey,
		Payload:       pb,
	}
}
