package store

import (
	"strings"
	"testing"
)

func TestMergeAndParseHeartbeatMetrics(t *testing.T) {
	merged := MergeHeartbeatMetrics(`{"note":"keep"}`, map[string]any{
		"gpu_mem":  64.5,
		"gpu_util": 81,
		"hostname": "skip-me",
	})
	got := ParseHeartbeatMetrics(merged)
	if got["gpu_mem"] != 64.5 || got["gpu_util"] != 81 {
		t.Fatalf("metrics = %#v", got)
	}
	if !strings.Contains(merged, `"note":"keep"`) {
		t.Fatalf("preserved metadata missing: %s", merged)
	}

	unchanged := MergeHeartbeatMetrics(merged, map[string]any{"provider": "cursor"})
	if unchanged != merged {
		t.Fatalf("identity heartbeat should keep metrics, got %s", unchanged)
	}
}
