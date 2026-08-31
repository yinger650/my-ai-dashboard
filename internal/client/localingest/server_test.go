package localingest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestHandleEventsDoesNotForward(t *testing.T) {
	srv, err := New(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), "127.0.0.1:7438")
	if err != nil {
		t.Fatal(err)
	}

	eventID := shared.NewID()
	env := event.Envelope{
		SchemaVersion: 1,
		EventID:       eventID,
		EventType:     event.TypeServiceState,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		ServiceKey:    "cursor",
		Payload:       json.RawMessage(`{"name":"Cursor Agent","type":"agent","state":"running","severity":"normal"}`),
	}
	raw, _ := json.Marshal(map[string]any{"events": []event.Envelope{env}})
	req := httptest.NewRequest(http.MethodPost, "/ingest/v1/events", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"accepted":1`) {
		t.Fatalf("body %s", rec.Body.String())
	}

	health := httptest.NewRecorder()
	srv.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), "ok") {
		t.Fatalf("health: %d %s", health.Code, health.Body.String())
	}
}

func TestTeeLogAppend(t *testing.T) {
	srv, err := New(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), "127.0.0.1:7438")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	srv.OnLogAppend = func(serviceKey, markdown, severity, occurredAt, source string) {
		got = append(got, serviceKey+"|"+markdown+"|"+severity+"|"+source)
	}
	env := event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeLogAppend,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		ServiceKey:    "cursor",
		Payload:       json.RawMessage(`{"markdown":"task failed","severity":"error","source":"cursor"}`),
	}
	raw, _ := json.Marshal(map[string]any{"events": []event.Envelope{env}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ingest/v1/events", strings.NewReader(string(raw))))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if len(got) != 1 || !strings.Contains(got[0], "task failed") {
		t.Fatalf("tee %v", got)
	}
}

func TestAdvertiseModeTee(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local-ingest.json")
	if err := writeAdvertise(path, "http://127.0.0.1:7438"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data["mode"] != "tee" {
		t.Fatalf("mode=%q body=%s", data["mode"], raw)
	}
	if data["url"] != "http://127.0.0.1:7438" {
		t.Fatalf("url=%q", data["url"])
	}
}

func TestOnEventReceivesCopies(t *testing.T) {
	srv, err := New(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), "127.0.0.1:7438")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	srv.OnEvent = func(env event.Envelope) {
		got = append(got, env.EventType+"|"+env.ServiceKey)
	}
	env := event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeServiceState,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		ServiceKey:    "cursor",
		Payload:       json.RawMessage(`{"name":"Cursor Agent","type":"agent","state":"running","severity":"normal","metadata":{"workspace":"/repo/demo"}}`),
	}
	raw, _ := json.Marshal(map[string]any{"events": []event.Envelope{env}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ingest/v1/events", strings.NewReader(string(raw))))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if len(got) != 1 || got[0] != "service.state|cursor" {
		t.Fatalf("got %v", got)
	}
}

func TestRejectsNonLoopbackListen(t *testing.T) {
	if _, err := New(slog.Default(), "0.0.0.0:7438"); err == nil {
		t.Fatal("expected error")
	}
}
