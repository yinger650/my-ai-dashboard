package server

import (
	"testing"
	"time"

	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

func TestMachineHealth(t *testing.T) {
	now := time.Now().UTC()
	seen := func(d time.Duration) *string {
		s := shared.FormatTime(now.Add(-d))
		return &s
	}

	tests := []struct {
		name   string
		m      *store.Machine
		metric *store.MetricSample
		want   string
	}{
		{"never seen", &store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30}, nil, "unknown"},
		{"online", &store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(10 * time.Second)}, nil, "online"},
		{"stale", &store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(70 * time.Second)}, nil, "stale"},
		{"offline", &store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(5 * time.Minute)}, nil, "offline"},
		{"virtual still online without host heartbeat", &store.Machine{Kind: "virtual", Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(5 * time.Minute)}, nil, "online"},
		{"virtual stale after 10m floor", &store.Machine{Kind: "virtual", Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(25 * time.Minute)}, nil, "stale"},
		{"virtual offline after 3x floor", &store.Machine{Kind: "virtual", Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(40 * time.Minute)}, nil, "offline"},
		{"disabled", &store.Machine{Enabled: false, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(5 * time.Second)}, nil, "disabled"},
		{
			"degraded on high cpu",
			&store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(5 * time.Second)},
			&store.MetricSample{CPUPercent: f64(97)},
			"degraded",
		},
		{
			"degraded on high disk",
			&store.Machine{Enabled: true, HeartbeatIntervalSeconds: 30, LastSeenAt: seen(5 * time.Second)},
			&store.MetricSample{RootDiskUsedBytes: i64(96), RootDiskTotalBytes: i64(100)},
			"degraded",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := machineHealth(tc.m, tc.metric, now)
			if got != tc.want {
				t.Errorf("machineHealth = %q, want %q", got, tc.want)
			}
		})
	}
}
