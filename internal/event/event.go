// Package event defines the AgentBoard event protocol: the common envelope,
// typed payloads and validation shared by board-server and board-client.
package event

import "encoding/json"

// Event types (spec 11.2).
const (
	TypeHeartbeat       = "machine.heartbeat"
	TypeMetricSample    = "metric.sample"
	TypePortSnapshot    = "machine.port_snapshot"
	TypeServiceSnapshot = "machine.service_snapshot"
	TypeServiceState    = "service.state"
	TypeStatusUpsert    = "status.upsert"
	TypeLogAppend       = "log.append"
	TypeLogPin          = "log.pin"
	TypeRunTransition   = "run.transition"
	TypeCollectorNotice = "collector.notice"
)

// KnownType reports whether t is a supported event type.
func KnownType(t string) bool {
	switch t {
	case TypeHeartbeat, TypeMetricSample, TypePortSnapshot, TypeServiceSnapshot,
		TypeServiceState, TypeStatusUpsert, TypeLogAppend, TypeLogPin,
		TypeRunTransition, TypeCollectorNotice:
		return true
	}
	return false
}

// Envelope is the common event wrapper (spec 11.1).
type Envelope struct {
	SchemaVersion int             `json:"schema_version"`
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    string          `json:"occurred_at"`
	BootID        string          `json:"boot_id,omitempty"`
	Sequence      *int64          `json:"sequence,omitempty"`
	ServiceKey    string          `json:"service_key,omitempty"`
	RunKey        string          `json:"run_key,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// Batch is the batched ingest request form.
type Batch struct {
	Events []json.RawMessage `json:"events"`
}

// Heartbeat is the payload for machine.heartbeat.
type Heartbeat struct {
	Hostname                 string         `json:"hostname"`
	OS                       string         `json:"os"`
	Arch                     string         `json:"arch"`
	CollectorVersion         string         `json:"collector_version"`
	HeartbeatIntervalSeconds int            `json:"heartbeat_interval_seconds"`
	UptimeSeconds            int64          `json:"uptime_seconds"`
	Metadata                 map[string]any `json:"metadata"`
}

// Filesystem is a mount entry in a metric sample.
type Filesystem struct {
	Mount      string `json:"mount"`
	UsedBytes  int64  `json:"used_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// MetricSample is the payload for metric.sample.
type MetricSample struct {
	CPUPercent         *float64                      `json:"cpu_percent"`
	Load1              *float64                      `json:"load1"`
	Load5              *float64                      `json:"load5"`
	Load15             *float64                      `json:"load15"`
	MemoryUsedBytes    *int64                        `json:"memory_used_bytes"`
	MemoryTotalBytes   *int64                        `json:"memory_total_bytes"`
	SwapUsedBytes      *int64                        `json:"swap_used_bytes"`
	SwapTotalBytes     *int64                        `json:"swap_total_bytes"`
	DiskReadBps        *float64                      `json:"disk_read_bps"`
	DiskWriteBps       *float64                      `json:"disk_write_bps"`
	NetworkRxBps       *float64                      `json:"network_rx_bps"`
	NetworkTxBps       *float64                      `json:"network_tx_bps"`
	RootDiskUsedBytes  *int64                        `json:"root_disk_used_bytes"`
	RootDiskTotalBytes *int64                        `json:"root_disk_total_bytes"`
	Filesystems        []Filesystem                  `json:"filesystems"`
	Interfaces         map[string]map[string]float64 `json:"interfaces"`
}

// ServiceState is the payload for service.state.
type ServiceState struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	State      string         `json:"state"`
	Summary    string         `json:"summary"`
	Severity   string         `json:"severity"`
	TTLSeconds *int           `json:"ttl_seconds"`
	Metadata   map[string]any `json:"metadata"`
}

// StatusItem is one entry of a status.upsert.
type StatusItem struct {
	Key           string          `json:"key"`
	Label         string          `json:"label"`
	Value         json.RawMessage `json:"value"`
	ValueType     string          `json:"value_type"`
	Unit          string          `json:"unit"`
	Severity      string          `json:"severity"`
	DisplayFormat string          `json:"display_format"`
	SortOrder     int             `json:"sort_order"`
}

// StatusUpsert is the payload for status.upsert.
type StatusUpsert struct {
	Items []StatusItem `json:"items"`
}

// LogPayload is the payload for log.append and log.pin.
type LogPayload struct {
	Markdown    string   `json:"markdown"`
	Severity    string   `json:"severity"`
	Source      string   `json:"source"`
	ArtifactIDs []string `json:"artifact_ids"`
}

// RunTransition is the payload for run.transition.
type RunTransition struct {
	ServiceName     string         `json:"service_name"`
	ServiceType     string         `json:"service_type"`
	Status          string         `json:"status"`
	Summary         string         `json:"summary"`
	StartedAt       string         `json:"started_at"`
	FinishedAt      string         `json:"finished_at"`
	Provider        string         `json:"provider"`
	ProviderAgentID string         `json:"provider_agent_id"`
	ProviderRunID   string         `json:"provider_run_id"`
	DurationMs      *int64         `json:"duration_ms"`
	InputTokens     *int64         `json:"input_tokens"`
	OutputTokens    *int64         `json:"output_tokens"`
	Metadata        map[string]any `json:"metadata"`
}

// CollectorNotice is the payload for collector.notice.
type CollectorNotice struct {
	Severity string         `json:"severity"`
	Code     string         `json:"code"`
	Markdown string         `json:"markdown"`
	Metadata map[string]any `json:"metadata"`
}

// ValidSeverity reports whether s is an allowed severity.
func ValidSeverity(s string) bool {
	switch s {
	case "normal", "info", "warning", "error", "unknown":
		return true
	}
	return false
}

// ValidServiceType reports whether t is an allowed service type.
func ValidServiceType(t string) bool {
	switch t {
	case "daemon", "scheduled", "job", "agent", "virtual":
		return true
	}
	return false
}

// ValidRunStatus reports whether s is an allowed run status.
func ValidRunStatus(s string) bool {
	switch s {
	case "queued", "running", "waiting_input", "blocked", "succeeded", "failed", "cancelled", "timed_out":
		return true
	}
	return false
}

// IsTerminal reports whether a run status is terminal.
func IsTerminal(s string) bool {
	switch s {
	case "succeeded", "failed", "cancelled", "timed_out":
		return true
	}
	return false
}

// AllowedTransition reports whether from->to is a legal run transition (spec 11.8).
func AllowedTransition(from, to string) bool {
	if IsTerminal(from) {
		return false
	}
	switch from {
	case "queued":
		return to == "running" || to == "cancelled"
	case "running":
		switch to {
		case "waiting_input", "blocked", "succeeded", "failed", "cancelled", "timed_out":
			return true
		}
	case "waiting_input":
		return to == "running" || to == "cancelled" || to == "timed_out"
	case "blocked":
		return to == "running" || to == "failed" || to == "cancelled" || to == "timed_out"
	}
	return false
}

// RunSeverity maps a run status to an event severity (spec 11.2).
func RunSeverity(status string) string {
	switch status {
	case "failed", "timed_out":
		return "error"
	case "blocked":
		return "warning"
	default:
		return "info"
	}
}
