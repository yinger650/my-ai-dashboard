package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"agentboard/internal/api"
)

// defaultSettings are the baseline settings merged with stored overrides.
func defaultSettings() map[string]any {
	return map[string]any{
		"board_title":           "AgentBoard Personal",
		"timezone":              "UTC",
		"poll_interval_seconds": 15,
		"allow_inline_images":   true,
		"cpu_warn":              cpuWarn,
		"cpu_err":               cpuErr,
		"mem_warn":              memWarn,
		"mem_err":               memErr,
		"disk_warn":             diskWarn,
		"disk_err":              diskErr,
	}
}

func (s *Server) settingString(r *http.Request, key, def string) string {
	v, err := s.st.GetSetting(r.Context(), key)
	if err != nil {
		return def
	}
	var out string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return def
	}
	return out
}

func (s *Server) settingInt(r *http.Request, key string, def int) int {
	v, err := s.st.GetSetting(r.Context(), key)
	if err != nil {
		return def
	}
	var n float64
	if err := json.Unmarshal([]byte(v), &n); err != nil {
		return def
	}
	return int(n)
}

func (s *Server) settingJSON(r *http.Request, key string) any {
	v, err := s.st.GetSetting(r.Context(), key)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil
	}
	return out
}

func (s *Server) mergedSettings(ctx context.Context) (map[string]any, error) {
	out := defaultSettings()
	stored, err := s.st.AllSettings(ctx)
	if err != nil {
		return nil, err
	}
	for k, raw := range stored {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err == nil {
			out[k] = v
		}
	}
	return out, nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	settings, err := s.mergedSettings(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	public := strings.TrimRight(s.cfg.PublicURL, "/")
	settings["public_url"] = public
	settings["board_txt_url"] = public + "/api/v1/board.txt"
	settings["ingest_url"] = public + "/ingest/v1/events"
	api.WriteData(w, rid, settings, nil)
}

var allowedSettingKeys = map[string]bool{
	"board_title": true, "timezone": true, "poll_interval_seconds": true, "allow_inline_images": true,
	"cpu_warn": true, "cpu_err": true, "mem_warn": true, "mem_err": true, "disk_warn": true, "disk_err": true,
	"raw_metric_retention_days": true, "event_retention_days": true, "artifact_quota_bytes": true,
	"board_layout": true,
}

func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	for k, v := range patch {
		if !allowedSettingKeys[k] {
			api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "unknown setting: "+k, rid)
			return
		}
		if err := s.st.SetSetting(r.Context(), k, string(v)); err != nil {
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
	}
	settings, _ := s.mergedSettings(r.Context())
	api.WriteData(w, rid, settings, nil)
}

func (s *Server) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	abnormal := r.URL.Query().Get("abnormal") == "1"
	cursor := r.URL.Query().Get("cursor")
	logs, err := s.st.ListAccessLogs(r.Context(), abnormal, cursor, 50)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	var next *string
	if len(logs) == 50 {
		c := logs[len(logs)-1].OccurredAt
		next = &c
	}
	api.WriteData(w, rid, logs, next)
}

func (s *Server) handleMaintenanceRun(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	deleted, err := s.st.DeleteExpiredSessions(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"expired_sessions_deleted": deleted}, nil)
}
