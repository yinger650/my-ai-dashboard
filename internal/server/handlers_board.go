package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentboard/internal/api"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

type sparkPoint struct {
	T   string   `json:"t"`
	CPU *float64 `json:"cpu"`
	Mem *float64 `json:"mem"`
	Net *float64 `json:"net"`
}

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
		recent, _ := s.st.RecentMachineLogs(ctx, m.ID, 3)
		spark, _ := s.st.Sparkline(ctx, m.ID, 30)

		points := make([]sparkPoint, 0, len(spark))
		for _, sp := range spark {
			var net *float64
			if sp.NetworkRxBps != nil || sp.NetworkTxBps != nil {
				v := val(sp.NetworkRxBps) + val(sp.NetworkTxBps)
				net = &v
			}
			points = append(points, sparkPoint{T: sp.OccurredAt, CPU: sp.CPUPercent, Mem: memPct(sp), Net: net})
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
			"recent_logs":       recent,
			"sparkline":         points,
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
	api.WriteData(w, rid, map[string]any{
		"title":           title,
		"machines":        board,
		"recent_abnormal": abnormal,
		"server_time":     shared.FormatTime(time.Now().UTC()),
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
		recent, _ := s.st.RecentMachineLogs(r.Context(), m.ID, 1)
		for _, l := range recent {
			fmt.Fprintf(&b, "  %s %s\n", strings.ToUpper(l.Severity), oneLine(l.Markdown))
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
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

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
