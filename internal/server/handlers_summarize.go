package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
	"agentboard/internal/summarize"
)

type summarizeRequest struct {
	Pin   bool `json:"pin"`
	Limit int  `json:"limit"`
}

func (s *Server) handleSummarizeLogs(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	svc, err := s.st.GetServiceByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	var req summarizeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	logs, err := s.st.ListServiceLogs(r.Context(), id, "", limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	bodies := make([]string, 0, len(logs))
	for i := len(logs) - 1; i >= 0; i-- {
		bodies = append(bodies, logs[i].Markdown)
	}
	md := summarize.Logs(svc.Name+" 日志总结", bodies)
	now := shared.FormatTime(shared.NowUTC())
	appendEnv := &event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeLogAppend,
		OccurredAt:    now,
		ServiceKey:    svc.ServiceKey,
		Payload:       mustJSON(event.LogPayload{Markdown: md, Severity: "info", Source: "summarize"}),
	}
	authz := store.IngestAuth{MachineID: svc.MachineID, ServiceID: &svc.ID}
	if _, err := s.st.IngestEvent(r.Context(), appendEnv, authz, now); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if req.Pin {
		pinEnv := &event.Envelope{
			SchemaVersion: 1,
			EventID:       shared.NewID(),
			EventType:     event.TypeLogPin,
			OccurredAt:    now,
			ServiceKey:    svc.ServiceKey,
			Payload:       mustJSON(event.LogPayload{Markdown: md, Severity: "info", Source: "summarize"}),
		}
		if _, err := s.st.IngestEvent(r.Context(), pinEnv, authz, now); err != nil {
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
	}
	api.WriteData(w, rid, map[string]any{"markdown": md, "pinned": req.Pin}, nil)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
