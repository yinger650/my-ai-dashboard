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

	mergedKeep := MergeHeartbeatMetrics(merged, map[string]any{"gpu_util": 90})
	gotKeep := ParseHeartbeatMetrics(mergedKeep)
	if gotKeep["gpu_util"] != 90 || gotKeep["gpu_mem"] != 64.5 {
		t.Fatalf("merge should keep other keys: %#v", gotKeep)
	}

	cleared := MergeHeartbeatMetrics(mergedKeep, map[string]any{"gpu_mem": nil})
	gotCleared := ParseHeartbeatMetrics(cleared)
	if _, ok := gotCleared["gpu_mem"]; ok {
		t.Fatalf("null should drop gpu_mem: %#v", gotCleared)
	}
	if gotCleared["gpu_util"] != 90 {
		t.Fatalf("other keys should remain: %#v", gotCleared)
	}
}
