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

func TestMachinePortsEmptyAndSnapshot(t *testing.T) {
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

	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/machines", sess.CSRFToken, map[string]any{
		"machine_key": "port-box", "name": "Port Box", "kind": "vm",
	})
	if code != 201 && code != 200 {
		t.Fatalf("create machine %d %+v", code, env)
	}
	var created struct {
		Machine store.Machine `json:"machine"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if created.Machine.ID == "" {
		t.Fatalf("machine id missing: %s", string(env.Data))
	}

	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/machines/"+created.Machine.ID+"/ports", "", nil)
	if code != 200 {
		t.Fatalf("empty ports %d %+v", code, env)
	}
	var empty struct {
		Ports []any `json:"ports"`
	}
	_ = json.Unmarshal(env.Data, &empty)
	if empty.Ports == nil {
		t.Fatal("ports should be empty list")
	}

	now := shared.FormatTime(shared.NowUTC())
	payload := map[string]any{"ports": []map[string]any{
		{"protocol": "tcp", "address": "127.0.0.1", "port": 8090, "process": "board-server"},
	}}
	res, err := st.IngestEvent(t.Context(), &event.Envelope{
		SchemaVersion: 1, EventID: shared.NewID(), EventType: event.TypePortSnapshot,
		OccurredAt: now, Payload: mustJSON(payload),
	}, store.IngestAuth{MachineID: created.Machine.ID, AutoCreateServices: true}, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("ingest ports: %v %+v", err, res)
	}

	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/machines/"+created.Machine.ID+"/ports", "", nil)
	if code != 200 {
		t.Fatalf("ports %d %+v", code, env)
	}
	var got struct {
		Ports []struct {
			Port    int    `json:"port"`
			Process string `json:"process"`
		} `json:"ports"`
		OccurredAt string `json:"occurred_at"`
	}
	_ = json.Unmarshal(env.Data, &got)
	if len(got.Ports) != 1 || got.Ports[0].Port != 8090 || got.Ports[0].Process != "board-server" || got.OccurredAt == "" {
		t.Fatalf("got %+v", got)
	}
}
