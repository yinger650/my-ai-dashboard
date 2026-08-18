package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentboard/internal/api"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func memPct(m *store.MetricSample) *float64 {
	if m == nil || m.MemoryUsedBytes == nil || m.MemoryTotalBytes == nil || *m.MemoryTotalBytes == 0 {
		return nil
	}
	p := float64(*m.MemoryUsedBytes) / float64(*m.MemoryTotalBytes) * 100
	return &p
}

func (s *Server) buildBoard(r *http.Request) ([]map[string]any, error) {
	ctx := r.Context()
	now := time.Now().UTC()
	machines, err := s.st.ListMachines(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(machines))
	for _, m := range machines {
		latest, _ := s.st.LatestMetric(ctx, m.ID)
		health, resSev := machineHealth(m, latest, now)
		counts, _ := s.st.ServiceSeverityCounts(ctx, m.ID)
		svcs, _ := s.st.ListServicesByMachine(ctx, m.ID)
		statuses, _ := s.st.ListStatusesByMachine(ctx, m.ID)
		pinned, _ := s.st.ListPinnedLogsByMachine(ctx, m.ID)
		recent, _ := s.st.ListMachineLogs(ctx, m.ID, "", 20)

		if svcs == nil {
			svcs = []*store.Service{}
		}
		if statuses == nil {
			statuses = []store.CurrentStatus{}
		}
		if pinned == nil {
			pinned = []store.PinnedLog{}
		}
		if recent == nil {
			recent = []store.LogEntry{}
		}

		services := make([]map[string]any, 0, len(svcs))
		for _, svc := range svcs {
			if !svc.Enabled {
				continue
			}
			services = append(services, map[string]any{
				"id":            svc.ID,
				"service_key":   svc.ServiceKey,
				"name":          svc.Name,
				"type":          svc.Type,
				"current_state": svc.CurrentState,
				"state_summary": svc.StateSummary,
				"severity":      svc.Severity,
				"last_seen_at":  svc.LastSeenAt,
			})
		}

		out = append(out, map[string]any{
			"id":                m.ID,
			"machine_key":       m.MachineKey,
			"name":              m.Name,
			"kind":              m.Kind,
			"health":            health,
			"resource_severity": resSev,
			"last_seen_at":      m.LastSeenAt,
			"os":                m.OS,
			"arch":              m.Arch,
			"latest_metric":     latest,
			"service_counts":    counts,
			"services":          services,
			"statuses":          statuses,
			"pinned_logs":       pinned,
			"recent_logs":       recent,
		})
	}
	return out, nil
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	board, err := s.buildBoard(r)
	if err != nil {
		s.log.Error("build board failed", "err", err)
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	since := shared.FormatTime(time.Now().UTC().Add(-time.Hour))
	abnormal, _ := s.st.AbnormalCountSince(r.Context(), since)
	title := s.settingString(r, "board_title", "AgentBoard Personal")
	poll := s.settingInt(r, "poll_interval_seconds", 15)
	layout := s.settingJSON(r, "board_layout")
	api.WriteData(w, rid, map[string]any{
		"title":                 title,
		"machines":              board,
		"recent_abnormal":       abnormal,
		"server_time":           shared.FormatTime(time.Now().UTC()),
		"poll_interval_seconds": poll,
		"layout":                layout,
		"public_url":            s.cfg.PublicURL,
	}, nil)
}

func (s *Server) handleBoardTxt(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	machines, err := s.st.ListMachines(r.Context(), false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	compact := r.URL.Query().Get("compact") == "1"
	filter := r.URL.Query().Get("machine")

	var b strings.Builder
	title := s.settingString(r, "board_title", "AgentBoard Personal")
	fmt.Fprintf(&b, "%s  %s\n\n", title, now.Format("2006-01-02 15:04:05 -07:00"))

	for _, m := range machines {
		if filter != "" && m.MachineKey != filter {
			continue
		}
		latest, _ := s.st.LatestMetric(r.Context(), m.ID)
		health, _ := machineHealth(m, latest, now)
		label := strings.ToUpper(health)
		if compact {
			fmt.Fprintf(&b, "[%s] %-20s %s\n", label, m.Name, lastSeenText(m, now))
			continue
		}
		fmt.Fprintf(&b, "[%s] %-20s %s\n", label, m.Name, metricSummary(latest, m, now))
		counts, _ := s.st.ServiceSeverityCounts(r.Context(), m.ID)
		fmt.Fprintf(&b, "  services: %d normal, %d warning, %d error\n", counts["normal"], counts["warning"], counts["error"])
		statuses, _ := s.st.ListStatusesByMachine(r.Context(), m.ID)
		if len(statuses) > 0 {
			parts := make([]string, 0, len(statuses))
			for _, st := range statuses {
				name := st.ServiceKey
				if name == "" {
					name = st.ServiceName
				}
				parts = append(parts, fmt.Sprintf("%s %s=%s", name, st.Label, statusValueText(st)))
			}
			fmt.Fprintf(&b, "  status: %s\n", strings.Join(parts, "  "))
		}
		pinned, _ := s.st.ListPinnedLogsByMachine(r.Context(), m.ID)
		for _, p := range pinned {
			fmt.Fprintf(&b, "  PIN %s %s\n", strings.ToUpper(p.Severity), oneLine(p.Markdown))
		}
		recent, _ := s.st.ListMachineLogs(r.Context(), m.ID, "", 5)
		for _, l := range recent {
			fmt.Fprintf(&b, "  %s %s\n", strings.ToUpper(l.Severity), oneLine(l.Markdown))
		}
		fmt.Fprintln(&b)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func statusValueText(st store.CurrentStatus) string {
	var v any
	if err := json.Unmarshal([]byte(st.ValueJSON), &v); err != nil {
		return st.ValueJSON
	}
	text := fmt.Sprint(v)
	if st.Unit != nil && *st.Unit != "" {
		return text + *st.Unit
	}
	return text
}

func metricSummary(latest *store.MetricSample, m *store.Machine, now time.Time) string {
	if latest == nil {
		return lastSeenText(m, now)
	}
	cpu, mem, disk := "--", "--", "--"
	if latest.CPUPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *latest.CPUPercent)
	}
	if p := memPct(latest); p != nil {
		mem = fmt.Sprintf("%.1f%%", *p)
	}
	if latest.RootDiskUsedBytes != nil && latest.RootDiskTotalBytes != nil && *latest.RootDiskTotalBytes > 0 {
		disk = fmt.Sprintf("%.1f%%", float64(*latest.RootDiskUsedBytes)/float64(*latest.RootDiskTotalBytes)*100)
	}
	return fmt.Sprintf("CPU %s  MEM %s  DISK %s  %s", cpu, mem, disk, lastSeenText(m, now))
}

func lastSeenText(m *store.Machine, now time.Time) string {
	if m.LastSeenAt == nil {
		return "never"
	}
	last, err := shared.ParseTime(*m.LastSeenAt)
	if err != nil {
		return "unknown"
	}
	d := now.Sub(last).Round(time.Second)
	return "last " + humanDuration(d)
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return s
}
