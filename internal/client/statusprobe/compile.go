// Package statusprobe compiles machine-level status_probe scripts and runs them.
package statusprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
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

// NoticeFunc reports a collector.notice-style message.
type NoticeFunc func(code, markdown string)

type probeMeta struct {
	Hash   string `json:"hash"`
	Kind   string `json:"kind,omitempty"`
	Intent string `json:"intent"`
	Path   string `json:"path"`
}

type httpArtifact struct {
	URL            string `json:"url"`
	Method         string `json:"method"`
	ExpectStatus   []int  `json:"expect_status"`
	ExpectContains string `json:"expect_contains,omitempty"`
}

// Compiler writes and trial-runs status_probe scripts under Dir.
type Compiler struct {
	Dir       string
	Provider  aiprovider.Provider
	AIEnabled bool
	Notice    NoticeFunc
}

// Prepare compiles or reuses scripts. Handwritten command entries skip the model.
func (c *Compiler) Prepare(ctx context.Context, probes []config.StatusProbe) []Ready {
	var out []Ready
	for _, p := range probes {
		if ctx.Err() != nil {
			return out
		}
		ready, ok := c.prepareOne(ctx, p)
		if ok {
			out = append(out, ready)
		}
	}
	return out
}

func (c *Compiler) prepareOne(ctx context.Context, p config.StatusProbe) (Ready, bool) {
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
	if len(p.Command) > 0 {
		if err := probe.CheckScript(p.Command[0]); err != nil {
			c.notice("status_probe_failed", "status_probe "+p.Key+": "+err.Error())
			return Ready{}, false
		}
		base.Command = append([]string(nil), p.Command...)
		return base, true
	}
	if c.Dir == "" {
		c.notice("status_probe_failed", "status_probe "+p.Key+": probe dir missing")
		return Ready{}, false
	}
	if err := os.MkdirAll(c.Dir, 0o750); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+": "+err.Error())
		return Ready{}, false
	}
	if p.Kind == config.StatusProbeHTTP {
		return c.prepareHTTP(ctx, p, base)
	}
	scriptPath := filepath.Join(c.Dir, p.Key+".sh")
	metaPath := filepath.Join(c.Dir, p.Key+".meta.json")
	sum := intentHash(p.Kind, p.Intent, p.Path)
	if cachedOK(scriptPath, metaPath, sum) {
		base.Command = []string{scriptPath}
		return base, true
	}
	if !c.AIEnabled || c.Provider == nil {
		if ready, ok := reuseOld(base, scriptPath); ok {
			return ready, true
		}
		c.notice("status_probe_skipped", "status_probe "+p.Key+": ai disabled and no compiled script")
		return Ready{}, false
	}
	body, err := c.generateScript(ctx, p)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		return reuseOld(base, scriptPath)
	}
	if err := validateGenerated(body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		return reuseOld(base, scriptPath)
	}
	tmpPath := scriptPath + ".new"
	if err := writeScript(tmpPath, body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" write: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(base, scriptPath)
	}
	out, _, err := probe.RunScript(ctx, []string{tmpPath}, timeout, 0)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(base, scriptPath)
	}
	if _, err := probe.ParseJSON(out); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial json: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(base, scriptPath)
	}
	if err := os.Rename(tmpPath, scriptPath); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" install: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(base, scriptPath)
	}
	_ = os.Chmod(scriptPath, scriptMode)
	if err := writeMeta(metaPath, probeMeta{Hash: sum, Kind: p.Kind, Intent: p.Intent, Path: p.Path}); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" meta: "+err.Error())
	}
	base.Command = []string{scriptPath}
	return base, true
}

func reuseOld(base Ready, scriptPath string) (Ready, bool) {
	metaPath := strings.TrimSuffix(scriptPath, ".sh") + ".meta.json"
	if probe.CheckScript(scriptPath) == nil && cachedKindOK(metaPath, base.Kind) {
		base.Command = []string{scriptPath}
		return base, true
	}
	return Ready{}, false
}

func (c *Compiler) generateScript(ctx context.Context, p config.StatusProbe) (string, error) {
	untrusted := "key=" + p.Key + "\nintent=" + p.Intent
	if p.Path != "" {
		untrusted += "\npath=" + p.Path
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

func (c *Compiler) prepareHTTP(ctx context.Context, p config.StatusProbe, base Ready) (Ready, bool) {
	artifactPath := filepath.Join(c.Dir, p.Key+".http.json")
	metaPath := filepath.Join(c.Dir, p.Key+".meta.json")
	sum := intentHash(p.Kind, p.Intent, p.Path)
	if metaHashOK(metaPath, sum) {
		if target, err := readHTTPTarget(artifactPath, p); err == nil {
			base.HTTP = target
			return base, true
		}
	}
	if !c.AIEnabled || c.Provider == nil {
		if target, err := readHTTPTarget(artifactPath, p); err == nil {
			base.HTTP = target
			return base, true
		}
		c.notice("status_probe_skipped", "status_probe "+p.Key+": ai disabled and no compiled http target")
		return Ready{}, false
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
		return reuseHTTP(base, artifactPath, p)
	}
	var artifact httpArtifact
	if err := json.Unmarshal([]byte(ExtractJSON(res.Text)), &artifact); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile json: "+err.Error())
		return reuseHTTP(base, artifactPath, p)
	}
	target, err := artifact.target(p)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		return reuseHTTP(base, artifactPath, p)
	}
	raw, _ := json.Marshal(artifact)
	tmpPath := artifactPath + ".new"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" write: "+err.Error())
		return reuseHTTP(base, artifactPath, p)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		c.notice("status_probe_failed", "status_probe "+p.Key+" chmod: "+err.Error())
		return reuseHTTP(base, artifactPath, p)
	}
	if err := os.Rename(tmpPath, artifactPath); err != nil {
		_ = os.Remove(tmpPath)
		c.notice("status_probe_failed", "status_probe "+p.Key+" install: "+err.Error())
		return reuseHTTP(base, artifactPath, p)
	}
	if err := writeMeta(metaPath, probeMeta{Hash: sum, Kind: p.Kind, Intent: p.Intent, Path: p.Path}); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" meta: "+err.Error())
	}
	base.HTTP = target
	return base, true
}

func reuseHTTP(base Ready, artifactPath string, p config.StatusProbe) (Ready, bool) {
	target, err := readHTTPTarget(artifactPath, p)
	if err != nil {
		return Ready{}, false
	}
	base.HTTP = target
	return base, true
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

func (c *Compiler) notice(code, md string) {
	if c.Notice != nil {
		c.Notice(code, md)
	}
}

func intentHash(kind, intent, path string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(intent) + "\x00" + strings.TrimSpace(path)))
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
