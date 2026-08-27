package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"agentboard/internal/api"
	"agentboard/internal/store"
)

// defaultSettings are the baseline settings merged with stored overrides.
func (s *Server) defaultSettings() map[string]any {
	eventDays, accessDays, metricDays := 30, 30, 30
	eventQuota := store.DefaultEventQuotaBytes
	artifactQuota := int64(5 * 1024 * 1024 * 1024)
	if s.cfg != nil {
		if s.cfg.EventRetention > 0 {
			eventDays = s.cfg.EventRetention
		}
		if s.cfg.AccessRetention > 0 {
			accessDays = s.cfg.AccessRetention
		}
		if s.cfg.RawMetricRetention > 0 {
			metricDays = s.cfg.RawMetricRetention
		}
		if s.cfg.EventQuotaBytes > 0 {
			eventQuota = s.cfg.EventQuotaBytes
		}
		if s.cfg.ArtifactQuotaBytes > 0 {
			artifactQuota = s.cfg.ArtifactQuotaBytes
		}
	}
	return map[string]any{
		"board_title":               "AgentBoard Personal",
		"timezone":                  "UTC",
		"poll_interval_seconds":     15,
		"allow_inline_images":       true,
		"cpu_warn":                  cpuWarn,
		"cpu_err":                   cpuErr,
		"mem_warn":                  memWarn,
		"mem_err":                   memErr,
		"disk_warn":                 diskWarn,
		"disk_err":                  diskErr,
		"raw_metric_retention_days": metricDays,
		"event_retention_days":      eventDays,
		"access_retention_days":     accessDays,
		"event_quota_bytes":         eventQuota,
		"artifact_quota_bytes":      artifactQuota,
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
	out := s.defaultSettings()
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
	"raw_metric_retention_days": true, "event_retention_days": true, "access_retention_days": true,
	"event_quota_bytes": true, "artifact_quota_bytes": true,
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
	res, err := s.st.ApplyRetention(r.Context(), s.retentionPolicy(r.Context()))
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{
		"expired_sessions_deleted": res.ExpiredSessions,
		"events_deleted":           res.EventsDeleted,
		"access_deleted":           res.AccessDeleted,
		"runs_deleted":             res.RunsDeleted,
		"quota_deleted":            res.QuotaDeleted,
		"events_bytes":             res.EventsBytes,
	}, nil)
}

func (s *Server) retentionPolicy(ctx context.Context) store.RetentionPolicy {
	p := store.RetentionPolicy{
		EventDays:  30,
		MetricDays: 30,
		AccessDays: 30,
		QuotaBytes: store.DefaultEventQuotaBytes,
	}
	if s.cfg != nil {
		if s.cfg.EventRetention > 0 {
			p.EventDays = s.cfg.EventRetention
		}
		if s.cfg.RawMetricRetention > 0 {
			p.MetricDays = s.cfg.RawMetricRetention
		}
		if s.cfg.AccessRetention > 0 {
			p.AccessDays = s.cfg.AccessRetention
		}
		if s.cfg.EventQuotaBytes > 0 {
			p.QuotaBytes = s.cfg.EventQuotaBytes
		}
	}
	if n := s.storedInt(ctx, "event_retention_days"); n > 0 {
		p.EventDays = n
	}
	if n := s.storedInt(ctx, "raw_metric_retention_days"); n > 0 {
		p.MetricDays = n
	}
	if n := s.storedInt(ctx, "access_retention_days"); n > 0 {
		p.AccessDays = n
	}
	if n := s.storedInt(ctx, "event_quota_bytes"); n > 0 {
		p.QuotaBytes = int64(n)
	}
	return p
}

func (s *Server) storedInt(ctx context.Context, key string) int {
	v, err := s.st.GetSetting(ctx, key)
	if err != nil {
		return 0
	}
	var n float64
	if err := json.Unmarshal([]byte(v), &n); err != nil {
		return 0
	}
	return int(n)
}
