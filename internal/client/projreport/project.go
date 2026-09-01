// Package projreport maps local-ingest agent copies onto proj-* services
// that board-client reports with its own Machine Token.
package projreport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"agentboard/internal/event"
)

var nonKey = regexp.MustCompile(`[^a-z0-9._-]+`)

// Out is one event board-client should enqueue for a host-side project.
type Out struct {
	Type       string
	ServiceKey string
	RunKey     string
	Payload    any
}

type metaWrap struct {
	Metadata map[string]any `json:"metadata"`
}

// WorkspaceOf reads payload.metadata.workspace from a raw event payload.
func WorkspaceOf(payload json.RawMessage) string {
	var wrap struct {
		Metadata struct {
			Workspace string `json:"workspace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(payload, &wrap); err != nil {
		return ""
	}
	return wrap.Metadata.Workspace
}

// KeyAndName turns a workspace path into proj-{dir} and a display name.
func KeyAndName(workspace string) (key, name string) {
	name = filepath.Base(filepath.Clean(strings.TrimSpace(workspace)))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "workspace"
	}
	key = slugKey("proj-"+name, "proj-workspace")
	return key, name
}

// Project rewrites an agent tee event onto the host-side proj-* service.
// Returns nil when the event has no workspace or is a machine-level type.
func Project(env event.Envelope) []Out {
	switch env.EventType {
	case event.TypeHeartbeat, event.TypeMetricSample, event.TypePortSnapshot, event.TypeServiceSnapshot:
		return nil
	}
	ws := WorkspaceOf(env.Payload)
	if ws == "" {
		return nil
	}
	key, name := KeyAndName(ws)
	if !event.ValidServiceKey(key) {
		return nil
	}

	payload := rewritePayload(env.EventType, env.Payload, name, ws)
	out := []Out{{
		Type:       env.EventType,
		ServiceKey: key,
		RunKey:     env.RunKey,
		Payload:    payload,
	}}
	if env.EventType == event.TypeServiceState {
		wsJSON, _ := json.Marshal(ws)
		out = append(out, Out{
			Type:       event.TypeStatusUpsert,
			ServiceKey: key,
			Payload: event.StatusUpsert{Items: []event.StatusItem{{
				Key:           "workspace",
				Label:         "目录",
				Value:         wsJSON,
				ValueType:     "string",
				Severity:      "info",
				DisplayFormat: "text",
				SortOrder:     40,
			}}},
		})
	}
	return out
}

func rewritePayload(eventType string, raw json.RawMessage, name, workspace string) any {
	var wrap metaWrap
	_ = json.Unmarshal(raw, &wrap)
	if wrap.Metadata == nil {
		wrap.Metadata = map[string]any{}
	}
	wrap.Metadata["workspace"] = workspace
	wrap.Metadata["project"] = name
	wrap.Metadata["host_project"] = true

	switch eventType {
	case event.TypeServiceState:
		st := event.ServiceState{}
		_ = json.Unmarshal(raw, &st)
		st.Name = name
		st.Type = "agent"
		st.Metadata = wrap.Metadata
		return st
	case event.TypeRunTransition:
		rt := event.RunTransition{}
		_ = json.Unmarshal(raw, &rt)
		rt.ServiceName = name
		rt.ServiceType = "agent"
		rt.Metadata = wrap.Metadata
		return rt
	case event.TypeCollectorNotice:
		n := event.CollectorNotice{}
		_ = json.Unmarshal(raw, &n)
		n.Metadata = wrap.Metadata
		return n
	case event.TypeLogAppend, event.TypeLogPin:
		lp := event.LogPayload{}
		_ = json.Unmarshal(raw, &lp)
		return lp
	default:
		return json.RawMessage(raw)
	}
}

func slugKey(raw, fallback string) string {
	s := nonKey.ReplaceAllString(strings.ToLower(raw), "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		s = fallback
	}
	if len(s) <= 64 && event.ValidServiceKey(s) {
		return s
	}
	sum := sha256.Sum256([]byte(raw))
	digest := hex.EncodeToString(sum[:8])[:8]
	keep := 64 - len(digest) - 1
	if keep < 1 {
		keep = 1
	}
	prefix := s
	if len(prefix) > keep {
		prefix = strings.TrimRight(prefix[:keep], "-._")
	}
	out := prefix + "-" + digest
	if len(out) > 64 {
		out = out[:64]
	}
	if !event.ValidServiceKey(out) {
		return (fallback + "-" + digest)[:64]
	}
	return out
}
