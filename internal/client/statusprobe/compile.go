// Package statusprobe compiles machine-level status_probe scripts and runs them.
package statusprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/probe"
)

const scriptMode = 0o755

// Ready is a compiled or handwritten probe ready for the side collect path.
type Ready struct {
	Key        string
	Kind       string
	Name       string
	Command    []string
	HTTP       *config.HTTPTarget
	Interval   time.Duration
	Timeout    time.Duration
	TTLSeconds int
}

// Preview is the last trial-run (or HTTP compile) result shown in config UI.
type Preview struct {
	OK     bool   `json:"ok"`
	Kind   string `json:"kind,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// NoticeFunc reports a collector.notice-style message.
type NoticeFunc func(code, markdown string)

type probeMeta struct {
	Hash          string   `json:"hash"`
	Kind          string   `json:"kind,omitempty"`
	Intent        string   `json:"intent"`
	IntentHistory []string `json:"intent_history,omitempty"`
	Path          string   `json:"path"`
}

type httpArtifact struct {
	URL            string `json:"url"`
	Method         string `json:"method"`
	ExpectStatus   []int  `json:"expect_status"`
	ExpectContains string `json:"expect_contains,omitempty"`
}

// Compiler writes and trial-runs status_probe scripts under Dir/nl/<key>/.
type Compiler struct {
	Dir       string // extensions root (nl/ and custom/ live here)
	LegacyDir string // old flat dirname(spool)/probes
	Provider  aiprovider.Provider
	AIEnabled bool
	Notice    NoticeFunc
}

// Prepare compiles or reuses scripts. Disabled probes are skipped.
// Handwritten command entries outside nl/<key> skip the model.
func (c *Compiler) Prepare(ctx context.Context, probes []config.StatusProbe) []Ready {
	var out []Ready
	for _, p := range probes {
		if ctx.Err() != nil {
			return out
		}
		if !p.IsEnabled() {
			continue
		}
		ready, _, ok := c.PrepareOne(ctx, p)
		if ok {
			out = append(out, ready)
		}
	}
	return out
}

// PrepareOne compiles one probe (even if disabled) and writes preview.json.
func (c *Compiler) PrepareOne(ctx context.Context, p config.StatusProbe) (Ready, Preview, bool) {
	ready, prev, ok := c.prepareOne(ctx, p)
	if prev.Kind == "" {
		prev.Kind = ready.Kind
		if prev.Kind == "" {
			if p.Kind == "" {
				prev.Kind = config.StatusProbeMetric
			} else {
				prev.Kind = p.Kind
			}
		}
	}
	c.writePreview(p.Key, prev)
	return ready, prev, ok
}

func (c *Compiler) prepareOne(ctx context.Context, p config.StatusProbe) (Ready, Preview, bool) {
	if p.Kind == "" {
		p.Kind = config.StatusProbeMetric
	}
	if p.Name == "" {
		p.Name = p.Key
	}
	if p.Kind != config.StatusProbeMetric && p.TTLSeconds <= 0 {
		p.TTLSeconds = 180
	}
	interval := p.Interval.Duration
	if interval <= 0 {
		interval = time.Minute
	}
	timeout := p.Timeout.Duration
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	base := Ready{
		Key: p.Key, Kind: p.Kind, Name: p.Name,
		Interval: interval, Timeout: timeout, TTLSeconds: p.TTLSeconds,
	}
	if len(p.Command) > 0 && !c.commandIsNL(p) {
		if err := probe.CheckScript(p.Command[0]); err != nil {
			msg := err.Error()
			c.notice("status_probe_failed", "status_probe "+p.Key+": "+msg)
			return Ready{}, Preview{Error: msg}, false
		}
		base.Command = append([]string(nil), p.Command...)
		return base, Preview{OK: true, Kind: p.Kind, Output: "handwritten " + p.Command[0]}, true
	}
	if c.Dir == "" {
		msg := "extensions dir missing"
		c.notice("status_probe_failed", "status_probe "+p.Key+": "+msg)
		return Ready{}, Preview{Error: msg}, false
	}
	if err := os.MkdirAll(c.nlDir(p.Key), 0o750); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+": "+err.Error())
		return Ready{}, Preview{Error: err.Error()}, false
	}
	if p.Kind == config.StatusProbeHTTP {
		return c.prepareHTTP(ctx, p, base)
	}
	scriptPath := c.scriptPath(p.Key)
	metaPath := c.metaPath(p.Key)
	sum := IntentHash(p)
	if cachedOK(scriptPath, metaPath, sum) {
		base.Command = []string{scriptPath}
		prev := readPreviewFile(c.previewPath(p.Key))
		if prev.Output == "" {
			prev = Preview{OK: true, Kind: p.Kind, Output: "cached " + scriptPath}
		}
		return base, prev, true
	}
	if !c.AIEnabled || c.Provider == nil {
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0]}, true
		}
		msg := "ai disabled and no compiled script"
		c.notice("status_probe_skipped", "status_probe "+p.Key+": "+msg)
		return Ready{}, Preview{Error: msg}, false
	}
	body, err := c.generateScript(ctx, p)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	if err := validateGenerated(body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	tmpPath := scriptPath + ".new"
	if err := writeScript(tmpPath, body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" write: "+err.Error())
		_ = os.Remove(tmpPath)
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	out, _, err := probe.RunScript(ctx, []string{tmpPath}, timeout, 0)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial: "+err.Error())
		_ = os.Remove(tmpPath)
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: "trial: " + err.Error()}, false
	}
	if _, err := probe.ParseJSON(out); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial json: "+err.Error())
		_ = os.Remove(tmpPath)
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: "trial json: " + err.Error()}, false
	}
	if err := os.Rename(tmpPath, scriptPath); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" install: "+err.Error())
		_ = os.Remove(tmpPath)
		if ready, ok := c.reuseOld(base, p); ok {
			return ready, Preview{OK: true, Kind: p.Kind, Output: "reused " + ready.Command[0], Error: err.Error()}, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	_ = os.Chmod(scriptPath, scriptMode)
	if err := writeMeta(metaPath, metaFrom(p, sum)); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" meta: "+err.Error())
	}
	base.Command = []string{scriptPath}
	return base, Preview{OK: true, Kind: p.Kind, Output: string(out)}, true
}

func (c *Compiler) reuseOld(base Ready, p config.StatusProbe) (Ready, bool) {
	scriptPath := c.scriptPath(p.Key)
	metaPath := c.metaPath(p.Key)
	if probe.CheckScript(scriptPath) == nil && cachedKindOK(metaPath, base.Kind) {
		base.Command = []string{scriptPath}
		return base, true
	}
	if c.LegacyDir == "" {
		return Ready{}, false
	}
	legacy := filepath.Join(c.LegacyDir, p.Key+".sh")
	legacyMeta := filepath.Join(c.LegacyDir, p.Key+".meta.json")
	if probe.CheckScript(legacy) == nil && cachedKindOK(legacyMeta, base.Kind) {
		base.Command = []string{legacy}
		return base, true
	}
	return Ready{}, false
}

func (c *Compiler) generateScript(ctx context.Context, p config.StatusProbe) (string, error) {
	untrusted := "key=" + p.Key + "\nintent=" + p.Intent
	if p.Path != "" {
		untrusted += "\npath=" + p.Path
	}
	if len(p.IntentHistory) > 0 {
		untrusted += "\nintent_history=" + strings.Join(p.IntentHistory, " | ")
	}
	task := "probe_script"
	if p.Kind == config.StatusProbeService {
		task = "service_probe_script"
	}
	res, err := c.Provider.Run(ctx, aiprovider.Request{
		Task:      task,
		Untrusted: untrusted,
		Timeout:   120 * time.Second,
		MaxRunes:  4000,
	})
	if err != nil {
		return "", err
	}
	script := ExtractScript(res.Text)
	if strings.TrimSpace(script) == "" {
		return "", fmt.Errorf("empty script")
	}
	if !strings.HasPrefix(strings.TrimSpace(script), "#!") {
		script = "#!/bin/sh\n" + script
	}
	return script, nil
}

func (c *Compiler) prepareHTTP(ctx context.Context, p config.StatusProbe, base Ready) (Ready, Preview, bool) {
	artifactPath := c.httpPath(p.Key)
	metaPath := c.metaPath(p.Key)
	sum := IntentHash(p)
	if metaHashOK(metaPath, sum) {
		if target, err := readHTTPTarget(artifactPath, p); err == nil {
			base.HTTP = target
			prev := readPreviewFile(c.previewPath(p.Key))
			if prev.Output == "" {
				prev = httpPreview(target, p)
			}
			return base, prev, true
		}
	}
	if !c.AIEnabled || c.Provider == nil {
		if target, err := readHTTPTarget(artifactPath, p); err == nil {
			base.HTTP = target
			return base, httpPreview(target, p), true
		}
		if ready, ok := c.reuseHTTP(base, p); ok {
			return ready, httpPreview(ready.HTTP, p), true
		}
		msg := "ai disabled and no compiled http target"
		c.notice("status_probe_skipped", "status_probe "+p.Key+": "+msg)
		return Ready{}, Preview{Error: msg}, false
	}
	res, err := c.Provider.Run(ctx, aiprovider.Request{
		Task:      "http_probe_config",
		Untrusted: "key=" + p.Key + "\nintent=" + p.Intent,
		WantJSON:  true,
		Timeout:   120 * time.Second,
		MaxRunes:  2000,
	})
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			prev := httpPreview(ready.HTTP, p)
			prev.Error = err.Error()
			return ready, prev, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	var artifact httpArtifact
	if err := json.Unmarshal([]byte(ExtractJSON(res.Text)), &artifact); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile json: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			prev := httpPreview(ready.HTTP, p)
			prev.Error = err.Error()
			return ready, prev, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	target, err := artifact.target(p)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			prev := httpPreview(ready.HTTP, p)
			prev.Error = err.Error()
			return ready, prev, true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	raw, _ := json.Marshal(artifact)
	tmpPath := artifactPath + ".new"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" write: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			return ready, httpPreview(ready.HTTP, p), true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		c.notice("status_probe_failed", "status_probe "+p.Key+" chmod: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			return ready, httpPreview(ready.HTTP, p), true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	if err := os.Rename(tmpPath, artifactPath); err != nil {
		_ = os.Remove(tmpPath)
		c.notice("status_probe_failed", "status_probe "+p.Key+" install: "+err.Error())
		if ready, ok := c.reuseHTTP(base, p); ok {
			return ready, httpPreview(ready.HTTP, p), true
		}
		return Ready{}, Preview{Error: err.Error()}, false
	}
	if err := writeMeta(metaPath, metaFrom(p, sum)); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" meta: "+err.Error())
	}
	base.HTTP = target
	return base, httpPreview(target, p), true
}

func (c *Compiler) reuseHTTP(base Ready, p config.StatusProbe) (Ready, bool) {
	if target, err := readHTTPTarget(c.httpPath(p.Key), p); err == nil {
		base.HTTP = target
		return base, true
	}
	if c.LegacyDir == "" {
		return Ready{}, false
	}
	legacy := filepath.Join(c.LegacyDir, p.Key+".http.json")
	if target, err := readHTTPTarget(legacy, p); err == nil {
		base.HTTP = target
		return base, true
	}
	return Ready{}, false
}

func readHTTPTarget(path string, p config.StatusProbe) (*config.HTTPTarget, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact httpArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, err
	}
	return artifact.target(p)
}

func (a httpArtifact) target(p config.StatusProbe) (*config.HTTPTarget, error) {
	u, err := url.Parse(strings.TrimSpace(a.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("http probe url must be an absolute http(s) URL")
	}
	if u.User != nil {
		return nil, fmt.Errorf("http probe url must not contain credentials")
	}
	method := strings.ToUpper(strings.TrimSpace(a.Method))
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "HEAD" {
		return nil, fmt.Errorf("http probe method must be GET or HEAD")
	}
	statuses := append([]int(nil), a.ExpectStatus...)
	if len(statuses) == 0 {
		statuses = []int{200}
	}
	for _, status := range statuses {
		if status < 100 || status > 599 {
			return nil, fmt.Errorf("http probe expect_status must be between 100 and 599")
		}
	}
	return &config.HTTPTarget{
		ServiceKey: p.Key, Name: p.Name, URL: u.String(), Method: method,
		ExpectStatus: statuses, ExpectContains: a.ExpectContains,
	}, nil
}

func httpPreview(target *config.HTTPTarget, p config.StatusProbe) Preview {
	if target == nil {
		return Preview{Kind: config.StatusProbeHTTP, Error: "compiled http target missing"}
	}
	raw, _ := json.MarshalIndent(httpArtifact{
		URL: target.URL, Method: target.Method,
		ExpectStatus: target.ExpectStatus, ExpectContains: target.ExpectContains,
	}, "", "  ")
	out := string(raw)
	if isLoopbackURL(target.URL) {
		timeout := p.Timeout.Duration
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		res := collector.ProbeHTTP(ctx, timeout, true, collector.HTTPTarget{
			ServiceKey: target.ServiceKey, Name: target.Name, URL: target.URL,
			Method: target.Method, ExpectStatus: target.ExpectStatus,
			ExpectContains: target.ExpectContains,
		})
		out += "\nprobe: " + res.Summary
		if !res.OK && res.Err != "" {
			out += " (" + res.Err + ")"
		}
	}
	return Preview{OK: true, Kind: config.StatusProbeHTTP, Output: out}
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func (c *Compiler) notice(code, md string) {
	if c.Notice != nil {
		c.Notice(code, md)
	}
}

func (c *Compiler) nlDir(key string) string {
	return filepath.Join(c.Dir, "nl", key)
}

func (c *Compiler) scriptPath(key string) string {
	return filepath.Join(c.nlDir(key), "probe.sh")
}

func (c *Compiler) httpPath(key string) string {
	return filepath.Join(c.nlDir(key), "http.json")
}

func (c *Compiler) metaPath(key string) string {
	return filepath.Join(c.nlDir(key), "meta.json")
}

func (c *Compiler) previewPath(key string) string {
	return filepath.Join(c.nlDir(key), "preview.json")
}

func (c *Compiler) commandIsNL(p config.StatusProbe) bool {
	if len(p.Command) == 0 || c.Dir == "" {
		return false
	}
	got := filepath.Clean(p.Command[0])
	want := filepath.Clean(c.scriptPath(p.Key))
	if absGot, err := filepath.Abs(got); err == nil {
		got = absGot
	}
	if absWant, err := filepath.Abs(want); err == nil {
		want = absWant
	}
	return got == want
}

func (c *Compiler) writePreview(key string, prev Preview) {
	if c.Dir == "" || key == "" {
		return
	}
	if err := os.MkdirAll(c.nlDir(key), 0o750); err != nil {
		return
	}
	b, err := json.MarshalIndent(prev, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.previewPath(key), b, 0o600)
}

// ReadPreview loads the last UI preview for a probe directory.
func ReadPreview(dir string) Preview {
	return readPreviewFile(filepath.Join(dir, "preview.json"))
}

func readPreviewFile(path string) Preview {
	b, err := os.ReadFile(path)
	if err != nil {
		return Preview{}
	}
	var p Preview
	if json.Unmarshal(b, &p) != nil {
		return Preview{Output: string(b)}
	}
	return p
}

// Built reports whether a compiled or handwritten artifact exists.
func Built(p config.StatusProbe, extRoot, legacyDir string) bool {
	if p.Kind == config.StatusProbeHTTP {
		dir := p.Dir
		if dir == "" {
			dir = config.NLRelDir(p.Key)
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(extRoot, dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "http.json")); err == nil {
			return true
		}
		if legacyDir != "" {
			_, err := os.Stat(filepath.Join(legacyDir, p.Key+".http.json"))
			return err == nil
		}
		return false
	}
	if len(p.Command) > 0 && probe.CheckScript(p.Command[0]) == nil {
		return true
	}
	dir := p.Dir
	if dir == "" {
		dir = config.NLRelDir(p.Key)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(extRoot, dir)
	}
	if probe.CheckScript(filepath.Join(dir, "probe.sh")) == nil {
		return true
	}
	if legacyDir != "" && probe.CheckScript(filepath.Join(legacyDir, p.Key+".sh")) == nil {
		return true
	}
	return false
}

// ApplyBuildResult writes dir/command onto the YAML probe after a successful compile.
func ApplyBuildResult(p *config.StatusProbe, ready Ready) {
	if p == nil || ready.Key == "" {
		return
	}
	p.Dir = config.NLRelDir(p.Key)
	if ready.Kind != config.StatusProbeHTTP && len(ready.Command) > 0 {
		p.Command = append([]string(nil), ready.Command...)
	}
	if ready.Kind == config.StatusProbeHTTP {
		p.Command = nil
	}
}

func metaFrom(p config.StatusProbe, sum string) probeMeta {
	return probeMeta{
		Hash: sum, Kind: p.Kind, Intent: p.Intent,
		IntentHistory: append([]string(nil), p.IntentHistory...), Path: p.Path,
	}
}

// IntentHash covers kind, current intent, path, and intent_history.
func IntentHash(p config.StatusProbe) string {
	kind := strings.TrimSpace(p.Kind)
	if kind == "" {
		kind = config.StatusProbeMetric
	}
	var b strings.Builder
	b.WriteString(kind)
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(p.Intent))
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(p.Path))
	for _, h := range p.IntentHistory {
		b.WriteByte(0)
		b.WriteString(strings.TrimSpace(h))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func cachedOK(scriptPath, metaPath, wantHash string) bool {
	if !metaHashOK(metaPath, wantHash) {
		return false
	}
	return probe.CheckScript(scriptPath) == nil
}

func metaHashOK(metaPath, wantHash string) bool {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var m probeMeta
	if json.Unmarshal(b, &m) != nil || m.Hash != wantHash {
		return false
	}
	return true
}

func cachedKindOK(metaPath, wantKind string) bool {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return wantKind == config.StatusProbeMetric
	}
	var m probeMeta
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	if m.Kind == "" {
		return wantKind == config.StatusProbeMetric
	}
	return m.Kind == wantKind
}

func writeMeta(path string, m probeMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func writeScript(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), scriptMode); err != nil {
		return err
	}
	return os.Chmod(path, scriptMode)
}

func validateGenerated(body string) error {
	low := strings.ToLower(body)
	for _, bad := range []string{
		"curl ", "wget ", "abp_m_", "agentboard_token", "abp_machine_token",
		"cursor_api_key", "/ingest/",
	} {
		if strings.Contains(low, bad) {
			return fmt.Errorf("generated script contains forbidden %q", strings.TrimSpace(bad))
		}
	}
	return nil
}

// ExtractScript pulls a shell script out of a model reply.
func ExtractScript(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "```"); i >= 0 {
		rest := text[i+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			lang := strings.TrimSpace(strings.ToLower(rest[:nl]))
			if lang == "" || lang == "sh" || lang == "bash" || lang == "shell" || lang == "zsh" {
				rest = rest[nl+1:]
			}
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		text = rest
	}
	return strings.TrimSpace(text)
}

// ExtractJSON strips an optional Markdown fence around a JSON object.
func ExtractJSON(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if nl := strings.Index(text, "\n"); nl >= 0 {
			text = text[nl+1:]
		}
		if i := strings.LastIndex(text, "```"); i >= 0 {
			text = text[:i]
		}
	}
	return strings.TrimSpace(text)
}

// NumericMeta maps probe JSON statuses that look like numbers.
func NumericMeta(key string, statuses []probe.Status) map[string]any {
	out := map[string]any{}
	for _, s := range statuses {
		k := strings.TrimSpace(s.Key)
		if k == "" {
			k = key
		}
		if n, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err == nil {
			out[k] = n
		}
	}
	return out
}

// NonNumericStatuses returns statuses whose values are not numbers.
func NonNumericStatuses(statuses []probe.Status) []probe.Status {
	var out []probe.Status
	for _, s := range statuses {
		if _, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err != nil {
			out = append(out, s)
		}
	}
	return out
}
