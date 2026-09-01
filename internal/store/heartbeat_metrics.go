package store

import (
	"encoding/json"
	"strings"
)

// HeartbeatMetricsKey is the machines.metadata_json field for numeric extras
// sent with machine.heartbeat (GPU util, VRAM, …).
const HeartbeatMetricsKey = "heartbeat_metrics"

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// MergeHeartbeatMetrics writes numeric heartbeat metadata into metadata_json.
// A heartbeat with no numeric extras leaves existing metrics unchanged.
func MergeHeartbeatMetrics(metadataJSON string, meta map[string]any) string {
	metrics := map[string]float64{}
	for k, v := range meta {
		if n, ok := asFloat(v); ok {
			metrics[k] = n
		}
	}
	if len(metrics) == 0 {
		if strings.TrimSpace(metadataJSON) == "" {
			return "{}"
		}
		return metadataJSON
	}
	obj := map[string]any{}
	if strings.TrimSpace(metadataJSON) != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &obj)
		if obj == nil {
			obj = map[string]any{}
		}
	}
	obj[HeartbeatMetricsKey] = metrics
	b, err := json.Marshal(obj)
	if err != nil {
		return metadataJSON
	}
	return string(b)
}

// ParseHeartbeatMetrics reads numeric extras stored under heartbeat_metrics.
func ParseHeartbeatMetrics(metadataJSON string) map[string]float64 {
	out := map[string]float64{}
	if strings.TrimSpace(metadataJSON) == "" {
		return out
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &obj); err != nil || obj == nil {
		return out
	}
	raw, ok := obj[HeartbeatMetricsKey]
	if !ok {
		return out
	}
	switch m := raw.(type) {
	case map[string]any:
		for k, v := range m {
			if n, ok := asFloat(v); ok {
				out[k] = n
			}
		}
	}
	return out
}
