package store

import (
	"encoding/json"
	"strings"
)

var servicePathKeys = []string{"path", "url"}

// MergeServiceMetadata copies path/url strings from an incoming service.state
// metadata map into services.metadata_json. Empty values do not clear a
// previously stored path.
func MergeServiceMetadata(metadataJSON string, meta map[string]any) string {
	obj := parseJSONObject(metadataJSON)
	changed := false
	for _, k := range servicePathKeys {
		s := metaString(meta, k)
		if s == "" {
			continue
		}
		if prev, _ := obj[k].(string); prev != s {
			obj[k] = s
			changed = true
		}
	}
	if !changed {
		if strings.TrimSpace(metadataJSON) == "" {
			return "{}"
		}
		return metadataJSON
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return metadataJSON
	}
	return string(b)
}

// ParseServicePath reads metadata.path, falling back to metadata.url
// (HTTP probes persist the target URL).
func ParseServicePath(metadataJSON string) string {
	obj := parseJSONObject(metadataJSON)
	if s := metaString(obj, "path"); s != "" {
		return s
	}
	return metaString(obj, "url")
}

func parseJSONObject(raw string) map[string]any {
	obj := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return obj
	}
	_ = json.Unmarshal([]byte(raw), &obj)
	if obj == nil {
		return map[string]any{}
	}
	return obj
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// FillPath copies metadata.path (or metadata.url) onto the JSON Path field.
func (s *Service) FillPath() {
	if s == nil {
		return
	}
	s.Path = ParseServicePath(s.MetadataJSON)
}
