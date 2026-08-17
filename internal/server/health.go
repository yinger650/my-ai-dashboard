package server

import (
	"time"

	"agentboard/internal/shared"
	"agentboard/internal/store"
)

// resource thresholds (spec 13.3 defaults).
const (
	cpuWarn  = 85.0
	cpuErr   = 95.0
	memWarn  = 90.0
	memErr   = 97.0
	diskWarn = 85.0
	diskErr  = 95.0
)

// machineHealth computes the health string for a machine given its latest
// metric sample and the current time.
func machineHealth(m *store.Machine, latest *store.MetricSample, now time.Time) (health string, resourceSeverity string) {
	if !m.Enabled {
		return "disabled", "unknown"
	}
	if m.LastSeenAt == nil {
		return "unknown", "unknown"
	}
	last, err := shared.ParseTime(*m.LastSeenAt)
	if err != nil {
		return "unknown", "unknown"
	}
	interval := time.Duration(m.HeartbeatIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	delta := now.Sub(last)

	switch {
	case delta <= 2*interval:
		health = "online"
	case delta <= 3*interval:
		return "stale", resourceSeverityOf(latest)
	default:
		return "offline", "unknown"
	}

	rs := resourceSeverityOf(latest)
	if rs == "warning" || rs == "error" {
		return "degraded", rs
	}
	return "online", rs
}

func resourceSeverityOf(m *store.MetricSample) string {
	if m == nil {
		return "unknown"
	}
	sev := "normal"
	raise := func(s string) {
		if severityRank(s) > severityRank(sev) {
			sev = s
		}
	}
	if m.CPUPercent != nil {
		if *m.CPUPercent >= cpuErr {
			raise("error")
		} else if *m.CPUPercent >= cpuWarn {
			raise("warning")
		}
	}
	if m.MemoryUsedBytes != nil && m.MemoryTotalBytes != nil && *m.MemoryTotalBytes > 0 {
		p := float64(*m.MemoryUsedBytes) / float64(*m.MemoryTotalBytes) * 100
		if p >= memErr {
			raise("error")
		} else if p >= memWarn {
			raise("warning")
		}
	}
	if m.RootDiskUsedBytes != nil && m.RootDiskTotalBytes != nil && *m.RootDiskTotalBytes > 0 {
		p := float64(*m.RootDiskUsedBytes) / float64(*m.RootDiskTotalBytes) * 100
		if p >= diskErr {
			raise("error")
		} else if p >= diskWarn {
			raise("warning")
		}
	}
	return sev
}

func severityRank(s string) int {
	switch s {
	case "error":
		return 3
	case "warning":
		return 2
	case "info", "unknown":
		return 1
	default:
		return 0
	}
}
