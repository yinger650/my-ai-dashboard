package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Edit is a cfgui save: overlay only the fields that were set.
type Edit struct {
	URL         string
	DisplayName string
	MachineKey  string
	Token       string // empty = keep existing

	// Features maps catalog id -> checked. Missing keys are left untouched
	// only when Features is nil. A non-nil map is applied in full.
	Features map[string]bool
	// Subs maps parent feature id -> sub id -> checked.
	Subs map[string]map[string]bool

	StatusProbes []StatusProbe
	HTTPTargets  []HTTPTarget
	ProbeScripts []ProbeScript
	WriteProbes  bool
	WriteHTTP    bool
	WriteScripts bool
}

// LoadDocument reads YAML into a node tree without applyDefaults.
// Missing files yield an empty mapping document.
func LoadDocument(path string) (doc *yaml.Node, cfg *Config, missing bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyDocument(), &Config{}, true, nil
		}
		return nil, nil, false, err
	}
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, false, err
	}
	if doc.Kind == 0 || (doc.Kind == yaml.DocumentNode && len(doc.Content) == 0) {
		doc = emptyDocument()
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, nil, false, err
	}
	return doc, &c, false, nil
}

func emptyDocument() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}},
	}
}

// DocRoot returns the mapping node of a YAML document.
func DocRoot(doc *yaml.Node) *yaml.Node {
	return rootMap(doc)
}

func rootMap(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			doc.Content = []*yaml.Node{m}
			return m
		}
		return doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		doc.Kind = yaml.MappingNode
		doc.Tag = "!!map"
	}
	return doc
}

// ApplyEdit overlays ed onto the YAML file at path and writes it atomically.
func ApplyEdit(path string, ed Edit) error {
	doc, _, missing, err := LoadDocument(path)
	if err != nil {
		return err
	}
	root := rootMap(doc)
	if missing {
		ensureSkeleton(root)
	}
	if Get(root, "version") == nil {
		setPath(root, []string{"version"}, scalarNode(1))
	}
	if ed.URL != "" {
		setPath(root, []string{"server", "url"}, scalarNode(ed.URL))
	}
	if ed.MachineKey != "" {
		setPath(root, []string{"machine", "key"}, scalarNode(ed.MachineKey))
	}
	if ed.DisplayName != "" {
		setPath(root, []string{"machine", "display_name"}, scalarNode(ed.DisplayName))
	}
	if strings.TrimSpace(ed.Token) != "" {
		setPath(root, []string{"server", "machine_token"}, scalarNode(strings.TrimSpace(ed.Token)))
	}
	if ed.Features != nil {
		applyFeatures(root, ed.Features, ed.Subs)
	}
	if ed.WriteProbes {
		setPath(root, []string{"machine", "status_probes"}, mustNode(ed.StatusProbes))
	}
	if ed.WriteHTTP {
		setPath(root, []string{"collectors", "http", "targets"}, mustNode(ed.HTTPTargets))
	}
	if ed.WriteScripts {
		setPath(root, []string{"collectors", "probes", "scripts"}, mustNode(ed.ProbeScripts))
	}
	return WriteDocument(path, doc)
}

func ensureSkeleton(root *yaml.Node) {
	setPath(root, []string{"version"}, scalarNode(1))
	setPath(root, []string{"server", "machine_token_env"}, scalarNode("ABP_MACHINE_TOKEN"))
	setPath(root, []string{"storage", "spool_path"}, scalarNode("/var/lib/agentboard-client/spool.db"))
}

func applyFeatures(root *yaml.Node, enabled map[string]bool, subs map[string]map[string]bool) {
	on := map[string]bool{}
	for k, v := range enabled {
		on[k] = v
	}
	if on[idDiscover] || on[idSummarizeAgentLogs] {
		on[idAI] = true
	}
	for _, f := range Catalog() {
		want, ok := on[f.ID]
		if !ok {
			continue
		}
		switch f.Kind {
		case KindSummarizeAgentLogs:
			applySummarizeAgentLogs(root, want)
		default:
			if len(f.Path) > 0 {
				setPath(root, f.Path, scalarNode(want))
			}
		}
		if f.ID == idDiscover {
			var sub map[string]bool
			if subs != nil {
				sub = subs[idDiscover]
			}
			applyDiscoverCommands(root, want, sub)
		}
	}
}

func applySummarizeAgentLogs(root *yaml.Node, on bool) {
	seq := Get(root, "ai", "summarize")
	var keep []*yaml.Node
	found := false
	if seq != nil && seq.Kind == yaml.SequenceNode {
		for _, it := range seq.Content {
			src := mapString(it, "source")
			if src == sourceAgentLogs || src == "" {
				found = true
				if on {
					keep = append(keep, it)
				}
				continue
			}
			keep = append(keep, it)
		}
	}
	if on && !found {
		keep = append(keep, mustNode(DefaultSummarizeAgentLogs()))
	}
	if keep == nil {
		keep = []*yaml.Node{}
	}
	setPath(root, []string{"ai", "summarize"}, seqNode(keep))
}

func applyDiscoverCommands(root *yaml.Node, parentOn bool, subChecked map[string]bool) {
	defaults := map[string]SubToggle{}
	for _, s := range DefaultDiscoverSubs() {
		defaults[s.ID] = s
	}
	seq := Get(root, "ai", "discover", "allow_commands")
	empty := seq == nil || seq.Kind != yaml.SequenceNode || len(seq.Content) == 0
	if empty && parentOn {
		var items []*yaml.Node
		for _, s := range DefaultDiscoverSubs() {
			if subChecked == nil || subChecked[s.ID] {
				items = append(items, mustNode(s.Seed))
			}
		}
		if len(items) > 0 {
			setPath(root, []string{"ai", "discover", "allow_commands"}, seqNode(items))
		}
		return
	}
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	if subChecked == nil {
		return
	}
	var keep []*yaml.Node
	seen := map[string]bool{}
	for _, it := range seq.Content {
		id := mapString(it, "id")
		if _, isDef := defaults[id]; isDef {
			seen[id] = true
			if subChecked[id] {
				keep = append(keep, it)
			}
			continue
		}
		keep = append(keep, it)
	}
	if parentOn {
		for _, s := range DefaultDiscoverSubs() {
			if subChecked[s.ID] && !seen[s.ID] {
				keep = append(keep, mustNode(s.Seed))
			}
		}
	}
	setPath(root, []string{"ai", "discover", "allow_commands"}, seqNode(keep))
}

// FeatureEnabled reports whether the YAML tree has the feature on.
func FeatureEnabled(root *yaml.Node, f Feature) bool {
	switch f.Kind {
	case KindSummarizeAgentLogs:
		seq := Get(root, "ai", "summarize")
		if seq == nil || seq.Kind != yaml.SequenceNode {
			return false
		}
		for _, it := range seq.Content {
			src := mapString(it, "source")
			if src == sourceAgentLogs || src == "" {
				return true
			}
		}
		return false
	default:
		n := Get(root, f.Path...)
		if n == nil || isNull(n) {
			if f.MissingMeans != nil {
				return *f.MissingMeans
			}
			return false
		}
		return isYAMLTrue(n)
	}
}

// SubEnabled reports whether a default allow_command id is present.
func SubEnabled(root *yaml.Node, parentID, subID string) bool {
	seq := Get(root, "ai", "discover", "allow_commands")
	if parentID != idDiscover || seq == nil || seq.Kind != yaml.SequenceNode {
		return false
	}
	for _, it := range seq.Content {
		if mapString(it, "id") == subID {
			return true
		}
	}
	return false
}

// Get walks a mapping path. Missing keys return nil.
func Get(root *yaml.Node, path ...string) *yaml.Node {
	cur := root
	for _, p := range path {
		if cur == nil || isNull(cur) || cur.Kind != yaml.MappingNode {
			return nil
		}
		cur = mapGet(cur, p)
	}
	return cur
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapString(m *yaml.Node, key string) string {
	n := mapGet(m, key)
	if n == nil {
		return ""
	}
	return n.Value
}

func setPath(root *yaml.Node, path []string, val *yaml.Node) {
	if len(path) == 0 || val == nil {
		return
	}
	cur := root
	for _, p := range path[:len(path)-1] {
		next := mapGet(cur, p)
		if next == nil || isNull(next) || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			mapSet(cur, p, next)
		}
		cur = next
	}
	mapSet(cur, path[len(path)-1], val)
}

func mapSet(m *yaml.Node, key string, val *yaml.Node) {
	if m.Kind != yaml.MappingNode {
		m.Kind = yaml.MappingNode
		m.Tag = "!!map"
		m.Content = nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

func scalarNode(v any) *yaml.Node {
	return mustNode(v)
}

func mustNode(v any) *yaml.Node {
	if v == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprint(v)}
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Value: strings.TrimSpace(string(b))}
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return &n
}

func seqNode(items []*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

func isNull(n *yaml.Node) bool {
	return n != nil && (n.Tag == "!!null" || n.Kind == yaml.ScalarNode && (n.Value == "null" || n.Value == "~" || n.Value == ""))
}

func isYAMLTrue(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(n.Value)) {
	case "true", "yes", "y", "on", "1":
		return true
	}
	return false
}

// WriteDocument encodes doc to path via rename (mode 0600).
func WriteDocument(path string, doc *yaml.Node) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	n := doc
	if doc != nil && doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		n = doc.Content[0]
		if n.HeadComment == "" && doc.HeadComment != "" {
			n.HeadComment = doc.HeadComment
		}
	}
	if err := enc.Encode(n); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".client.yaml.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// SpoolPathFromDoc returns storage.spool_path or the client default.
func SpoolPathFromDoc(root *yaml.Node) string {
	n := Get(root, "storage", "spool_path")
	if n != nil && strings.TrimSpace(n.Value) != "" {
		return n.Value
	}
	return "/var/lib/agentboard-client/spool.db"
}

// FormatDuration is a helper for UI interval fields.
func FormatDuration(d Duration) string {
	if d.Duration == 0 {
		return ""
	}
	return d.Duration.String()
}

// ParseExpectStatus parses "200" or "200,204".
func ParseExpectStatus(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}
