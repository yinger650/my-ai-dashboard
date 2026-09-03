package config

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// SeenFeaturesKey is the spool client_state key for reviewed catalog ids.
const SeenFeaturesKey = "seen_features"

// Feature groups shown in config tui/web.
const (
	GroupHostMetrics  = "主机指标"
	GroupHostServices = "主机服务"
	GroupProbe        = "探测"
	GroupAI           = "AI"
)

const (
	idSummarizeAgentLogs = "ai.summarize.agent_logs"
	idDiscover           = "ai.discover"
	idAI                 = "ai"
	idLocalIngest        = "local_ingest"
	sourceAgentLogs      = "agent_logs"
)

// FeatureKind selects how a catalog entry maps onto YAML.
type FeatureKind int

const (
	// KindBool is a YAML boolean at Path.
	KindBool FeatureKind = iota
	// KindSummarizeAgentLogs is enabled when ai.summarize contains source=agent_logs.
	KindSummarizeAgentLogs
)

// SubToggle is a default bundled option under a feature (e.g. discover whitelist).
type SubToggle struct {
	ID    string
	Title string
	Seed  AllowCmd
}

// Feature is one built-in toggle in the client catalog.
type Feature struct {
	ID           string
	Title        string
	Group        string
	Kind         FeatureKind
	Path         []string
	DefaultOn    bool
	MissingMeans *bool // if set, a missing YAML path means this value (local_ingest)
	Subs         []SubToggle
}

// Catalog is the built-in feature list. New binaries append stable ids here.
func Catalog() []Feature {
	t, f := true, false
	return []Feature{
		{ID: "cpu", Title: "CPU", Group: GroupHostMetrics, Path: []string{"collectors", "cpu"}, DefaultOn: true},
		{ID: "memory", Title: "内存", Group: GroupHostMetrics, Path: []string{"collectors", "memory"}, DefaultOn: true},
		{ID: "filesystems", Title: "文件系统", Group: GroupHostMetrics, Path: []string{"collectors", "filesystems", "enabled"}, DefaultOn: true},
		{ID: "disk_io", Title: "磁盘 IO", Group: GroupHostMetrics, Path: []string{"collectors", "disk_io", "enabled"}, DefaultOn: true},
		{ID: "network", Title: "网络", Group: GroupHostMetrics, Path: []string{"collectors", "network", "enabled"}, DefaultOn: true},
		{ID: "ports", Title: "监听端口", Group: GroupHostMetrics, Path: []string{"collectors", "ports", "enabled"}, DefaultOn: true},
		{ID: "docker", Title: "Docker", Group: GroupHostServices, Path: []string{"collectors", "docker", "enabled"}, DefaultOn: true},
		{ID: "cron", Title: "Cron", Group: GroupHostServices, Path: []string{"collectors", "cron", "enabled"}, DefaultOn: true},
		{ID: "nginx", Title: "Nginx", Group: GroupHostServices, Path: []string{"collectors", "nginx", "enabled"}, DefaultOn: true},
		{ID: "systemd", Title: "systemd", Group: GroupHostServices, Path: []string{"collectors", "systemd", "enabled"}},
		{ID: idLocalIngest, Title: "本机 ingest", Group: GroupHostServices, Path: []string{"local_ingest", "enabled"}, DefaultOn: true, MissingMeans: &t},
		{ID: "update", Title: "自动升级", Group: GroupHostServices, Path: []string{"update", "enabled"}, MissingMeans: &f},
		{ID: "cursor_agent", Title: "Cursor transcript", Group: GroupProbe, Path: []string{"collectors", "cursor_agent", "enabled"}},
		{ID: "http", Title: "HTTP 网站探测", Group: GroupProbe, Path: []string{"collectors", "http", "enabled"}},
		{ID: "probes", Title: "自定义 probe 脚本", Group: GroupProbe, Path: []string{"collectors", "probes", "enabled"}},
		{ID: idAI, Title: "AI 总开关", Group: GroupAI, Path: []string{"ai", "enabled"}},
		{ID: idSummarizeAgentLogs, Title: "Agent 日志总结", Group: GroupAI, Kind: KindSummarizeAgentLogs},
		{ID: idDiscover, Title: "AI 主机巡检", Group: GroupAI, Path: []string{"ai", "discover", "enabled"}, Subs: DefaultDiscoverSubs()},
	}
}

// DefaultDiscoverSubs is the bundled AI inspect whitelist.
func DefaultDiscoverSubs() []SubToggle {
	return []SubToggle{
		{
			ID:    "unit_status",
			Title: "systemctl status",
			Seed: AllowCmd{
				ID:   "unit_status",
				Argv: []string{"systemctl", "status", "--no-pager", "-n", "50", "{unit}"},
			},
		},
		{
			ID:    "unit_journal",
			Title: "journalctl -u",
			Seed: AllowCmd{
				ID:   "unit_journal",
				Argv: []string{"journalctl", "--no-pager", "-n", "200", "-u", "{unit}"},
			},
		},
		{
			ID:    "read_file",
			Title: "读取白名单文件",
			Seed: AllowCmd{
				ID:         "read_file",
				Argv:       []string{"cat", "{path}"},
				AllowPaths: []string{"/var/log/**", "/etc/agentboard/**"},
			},
		},
	}
}

// DefaultSummarizeAgentLogs is the seed row for Agent 日志总结.
func DefaultSummarizeAgentLogs() AISummarize {
	return AISummarize{
		Source:     sourceAgentLogs,
		ServiceKey: "ai-agent-digest",
		Name:       "Agent 日志总结",
		Interval:   Duration{15 * time.Minute},
		MinNewLogs: 3,
		Prompt:     "重点关注任务失败与卡住的原因",
	}
}

// FeatureByID returns a catalog entry.
func FeatureByID(id string) (Feature, bool) {
	for _, f := range Catalog() {
		if f.ID == id {
			return f, true
		}
	}
	return Feature{}, false
}

// AllCatalogIDs returns feature ids and parent.sub ids.
func AllCatalogIDs() []string {
	var ids []string
	for _, f := range Catalog() {
		ids = append(ids, f.ID)
		for _, s := range f.Subs {
			ids = append(ids, f.ID+"."+s.ID)
		}
	}
	return ids
}

// ParseSeenIDs reads the JSON array stored in client_state.
func ParseSeenIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var wrap struct {
		IDs []string `json:"ids"`
	}
	if json.Unmarshal([]byte(raw), &wrap) == nil && len(wrap.IDs) > 0 {
		return uniqStrings(wrap.IDs)
	}
	var flat []string
	if json.Unmarshal([]byte(raw), &flat) == nil {
		return uniqStrings(flat)
	}
	return nil
}

// EncodeSeenIDs serializes catalog ids for client_state.
func EncodeSeenIDs(ids []string) string {
	b, _ := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: uniqStrings(ids)})
	return string(b)
}

// SeenSet is a set of reviewed catalog ids.
func SeenSet(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

// UnseenIDs returns catalog ids not present in seen.
func UnseenIDs(seen []string) []string {
	s := SeenSet(seen)
	var out []string
	for _, id := range AllCatalogIDs() {
		if !s[id] {
			out = append(out, id)
		}
	}
	return out
}

// UnseenTitles lists unique feature titles for unseen ids (subs roll up to parent).
func UnseenTitles(seen []string) []string {
	s := SeenSet(seen)
	var titles []string
	have := map[string]bool{}
	for _, f := range Catalog() {
		mark := !s[f.ID]
		if !mark {
			for _, sub := range f.Subs {
				if !s[f.ID+"."+sub.ID] {
					mark = true
					break
				}
			}
		}
		if mark && !have[f.Title] {
			have[f.Title] = true
			titles = append(titles, f.Title)
		}
	}
	return titles
}

// PresentIDs are catalog ids already represented in cfg (enabled bools, existing lists).
func PresentIDs(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	var ids []string
	for _, f := range Catalog() {
		if featurePresent(cfg, f) {
			ids = append(ids, f.ID)
		}
		if f.ID == idDiscover {
			have := map[string]bool{}
			for _, c := range cfg.AI.Discover.AllowCommands {
				have[c.ID] = true
			}
			for _, sub := range f.Subs {
				if have[sub.ID] {
					ids = append(ids, f.ID+"."+sub.ID)
				}
			}
		}
	}
	return ids
}

func featurePresent(cfg *Config, f Feature) bool {
	switch f.ID {
	case "cpu":
		return cfg.Collectors.CPU
	case "memory":
		return cfg.Collectors.Memory
	case "filesystems":
		return cfg.Collectors.Filesystems.Enabled
	case "disk_io":
		return cfg.Collectors.DiskIO.Enabled
	case "network":
		return cfg.Collectors.Network.Enabled
	case "ports":
		return cfg.Collectors.Ports.Enabled
	case "docker":
		return cfg.Collectors.Docker.Enabled
	case "cron":
		return cfg.Collectors.Cron.Enabled
	case "nginx":
		return cfg.Collectors.Nginx.Enabled
	case "systemd":
		return cfg.Collectors.Systemd.Enabled
	case idLocalIngest:
		return cfg.LocalIngestOn()
	case "update":
		return cfg.Update.Enabled
	case "cursor_agent":
		return cfg.Collectors.CursorAgent.Enabled
	case "http":
		return cfg.Collectors.HTTP.Enabled
	case "probes":
		return cfg.Collectors.Probes.Enabled
	case idAI:
		return cfg.AI.Enabled
	case idSummarizeAgentLogs:
		for _, s := range cfg.AI.Summarize {
			if s.Source == sourceAgentLogs || s.Source == "" {
				return true
			}
		}
		return false
	case idDiscover:
		return cfg.AI.Discover.Enabled
	default:
		return false
	}
}

// EffectiveSeen returns stored ids, or PresentIDs when the user has never reviewed.
func EffectiveSeen(stored []string, cfg *Config) []string {
	if len(stored) > 0 {
		return stored
	}
	return PresentIDs(cfg)
}

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
