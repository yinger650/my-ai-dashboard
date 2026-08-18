package server

import (
	"encoding/json"
	"io"
	"net/http"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

const maxBatchBytes = 512 * 1024

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	tok := tokenFrom(r.Context())
	if tok == nil {
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{
		"server_time": shared.FormatTime(shared.NowUTC()),
		"scope":       tok.Scope,
		"machine_id":  tok.MachineID,
		"service_id":  tok.ServiceID,
	}, nil)
}

// resolveIngestAuth derives the ingest authorization context from the token.
func (s *Server) resolveIngestAuth(r *http.Request, tok *store.Token) (*store.IngestAuth, string) {
	switch tok.Scope {
	case auth.ScopeMachine:
		if tok.MachineID == nil {
			return nil, "token has no machine"
		}
		m, err := s.st.GetMachineByID(r.Context(), *tok.MachineID)
		if err != nil {
			return nil, "machine not found"
		}
		if m.DeletedAt != nil || !m.Enabled {
			return nil, "machine disabled"
		}
		return &store.IngestAuth{MachineID: m.ID, AutoCreateServices: m.AutoCreateServices}, ""
	case auth.ScopeService:
		if tok.ServiceID == nil {
			return nil, "token has no service"
		}
		svc, err := s.st.GetServiceByID(r.Context(), *tok.ServiceID)
		if err != nil {
			return nil, "service not found"
		}
		return &store.IngestAuth{MachineID: svc.MachineID, ServiceID: &svc.ID, AutoCreateServices: false}, ""
	default:
		return nil, "viewer token cannot ingest"
	}
}

func (s *Server) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	tok := tokenFrom(r.Context())
	if tok == nil {
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	if tok.Scope == auth.ScopeViewer {
		s.markAbnormal(r, "forbidden", "viewer cannot ingest")
		api.WriteError(w, http.StatusForbidden, api.CodeForbidden, "forbidden", rid)
		return
	}

	ingestAuth, reason := s.resolveIngestAuth(r, tok)
	if ingestAuth == nil {
		s.markAbnormal(r, "forbidden", reason)
		api.WriteError(w, http.StatusForbidden, api.CodeForbidden, "forbidden", rid)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBatchBytes+1))
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "read error", rid)
		return
	}
	if len(body) > maxBatchBytes {
		s.markAbnormal(r, "payload_too_large", "batch too large")
		api.WriteError(w, http.StatusRequestEntityTooLarge, api.CodePayloadTooLarge, "payload too large", rid)
		return
	}
	if ai := accessFrom(r.Context()); ai != nil {
		ai.bytesIn = int64(len(body))
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		s.markAbnormal(r, "invalid_json", "cannot parse json")
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}

	var rawEvents []json.RawMessage
	if evts, ok := top["events"]; ok {
		if err := json.Unmarshal(evts, &rawEvents); err != nil {
			api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid events array", rid)
			return
		}
	} else {
		rawEvents = []json.RawMessage{json.RawMessage(body)}
	}

	if len(rawEvents) == 0 || len(rawEvents) > 200 {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "events must be 1..200", rid)
		return
	}

	receivedAt := shared.FormatTime(shared.NowUTC())
	type result struct {
		EventID string `json:"event_id"`
		Status  string `json:"status"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	}
	results := make([]result, 0, len(rawEvents))
	var accepted, duplicates, rejectedCount int

	for _, raw := range rawEvents {
		var env event.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			results = append(results, result{Status: "rejected", Code: api.CodeValidationFailed, Message: "invalid event json"})
			rejectedCount++
			continue
		}
		if env.SchemaVersion != 1 {
			results = append(results, result{EventID: env.EventID, Status: "rejected", Code: api.CodeValidationFailed, Message: "schema_version must be 1"})
			rejectedCount++
			continue
		}
		res, err := s.st.IngestEvent(r.Context(), &env, *ingestAuth, receivedAt)
		if err != nil {
			s.log.Error("ingest event failed", "err", err, "event_id", env.EventID)
			results = append(results, result{EventID: env.EventID, Status: "rejected", Code: api.CodeInternalError, Message: "internal error"})
			rejectedCount++
			continue
		}
		switch res.Status {
		case "accepted":
			accepted++
		case "duplicate":
			duplicates++
		case "rejected":
			rejectedCount++
			if res.Abnormal {
				if ai := accessFrom(r.Context()); ai != nil {
					ai.isAbnormal = true
					ai.result = "rejected"
					ai.reason = res.Code
				}
			}
		}
		results = append(results, result{EventID: env.EventID, Status: res.Status, Code: res.Code, Message: res.Message})
	}

	api.WriteData(w, rid, map[string]any{
		"accepted":    accepted,
		"duplicates":  duplicates,
		"rejected":    rejectedCount,
		"results":     results,
		"server_time": shared.FormatTime(shared.NowUTC()),
	}, nil)
}
