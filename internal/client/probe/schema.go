package probe

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentboard/internal/event"
)

// Result is the narrow stdout schema for format=json probes.
type Result struct {
	State          string    `json:"state"`
	Summary        string    `json:"summary"`
	Severity       string    `json:"severity"`
	Statuses       []Status  `json:"statuses"`
	Logs           []LogLine `json:"logs"`
	PinnedMarkdown string    `json:"pinned_markdown"`
	Truncated      bool      `json:"truncated,omitempty"`
}

// Status is one status.upsert item from a probe.
type Status struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Unit     string `json:"unit"`
	Severity string `json:"severity"`
}

// LogLine is one rolling log from a probe.
type LogLine struct {
	Markdown string `json:"markdown"`
	Severity string `json:"severity"`
}

// ParseJSON decodes the narrow schema. Extra envelope-like fields are ignored.
func ParseJSON(raw []byte) (Result, error) {
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf("probe json: %w", err)
	}
	if r.State == "" {
		r.State = "running"
	}
	if r.Severity == "" {
		r.Severity = "normal"
	}
	if !event.ValidSeverity(r.Severity) {
		r.Severity = "unknown"
	}
	for i := range r.Statuses {
		if r.Statuses[i].Severity == "" {
			r.Statuses[i].Severity = "normal"
		}
		if !event.ValidSeverity(r.Statuses[i].Severity) {
			r.Statuses[i].Severity = "unknown"
		}
		if r.Statuses[i].Key == "" {
			r.Statuses[i].Key = fmt.Sprintf("item_%d", i)
		}
	}
	for i := range r.Logs {
		if r.Logs[i].Severity == "" {
			r.Logs[i].Severity = "info"
		}
		if !event.ValidSeverity(r.Logs[i].Severity) {
			r.Logs[i].Severity = "info"
		}
	}
	return r, nil
}

// MappedEvent is a projected Board event (not yet enveloped).
type MappedEvent struct {
	Type       string
	ServiceKey string
	Payload    any
}

// MapJSON maps a parsed result onto the owning service_key only.
func MapJSON(serviceKey, name string, ttl int, r Result, pinHash string) (events []MappedEvent, newHash string) {
	if name == "" {
		name = serviceKey
	}
	sev := r.Severity
	st := r.State
	if st == "" {
		st = "running"
	}
	events = append(events, MappedEvent{
		Type:       event.TypeServiceState,
		ServiceKey: serviceKey,
		Payload: event.ServiceState{
			Name: name, Type: "virtual", State: st,
			Summary: r.Summary, Severity: sev, TTLSeconds: ttlPtr(ttl),
		},
	})
	if len(r.Statuses) > 0 {
		items := make([]event.StatusItem, 0, len(r.Statuses))
		for i, s := range r.Statuses {
			val, _ := json.Marshal(s.Value)
			label := s.Label
			if label == "" {
				label = s.Key
			}
			items = append(items, event.StatusItem{
				Key: s.Key, Label: label, Value: val, ValueType: "string",
				Unit: s.Unit, Severity: s.Severity, DisplayFormat: "text", SortOrder: (i + 1) * 10,
			})
		}
		events = append(events, MappedEvent{
			Type: event.TypeStatusUpsert, ServiceKey: serviceKey,
			Payload: event.StatusUpsert{Items: items},
		})
	}
	for _, lg := range r.Logs {
		if strings.TrimSpace(lg.Markdown) == "" {
			continue
		}
		events = append(events, MappedEvent{
			Type: event.TypeLogAppend, ServiceKey: serviceKey,
			Payload: event.LogPayload{Markdown: lg.Markdown, Severity: lg.Severity, Source: "probe"},
		})
	}
	if md := strings.TrimSpace(r.PinnedMarkdown); md != "" {
		sum := hashMarkdown(md)
		if sum != pinHash {
			events = append(events, MappedEvent{
				Type: event.TypeLogPin, ServiceKey: serviceKey,
				Payload: event.LogPayload{Markdown: md, Severity: sev, Source: "probe"},
			})
			newHash = sum
		} else {
			newHash = pinHash
		}
	} else {
		newHash = pinHash
	}
	return events, newHash
}

func ttlPtr(n int) *int {
	if n <= 0 {
		return nil
	}
	v := n
	return &v
}
