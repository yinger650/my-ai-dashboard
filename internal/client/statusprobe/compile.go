// Package statusprobe compiles machine-level status_probe scripts and runs them.
package statusprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	Key      string
	Command  []string
	Interval time.Duration
	Timeout  time.Duration
}

// NoticeFunc reports a collector.notice-style message.
type NoticeFunc func(code, markdown string)

type probeMeta struct {
	Hash   string `json:"hash"`
	Intent string `json:"intent"`
	Path   string `json:"path"`
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
	interval := p.Interval.Duration
	if interval <= 0 {
		interval = time.Minute
	}
	timeout := p.Timeout.Duration
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if len(p.Command) > 0 {
		if err := probe.CheckScript(p.Command[0]); err != nil {
			c.notice("status_probe_failed", "status_probe "+p.Key+": "+err.Error())
			return Ready{}, false
		}
		return Ready{Key: p.Key, Command: append([]string(nil), p.Command...), Interval: interval, Timeout: timeout}, true
	}
	if c.Dir == "" {
		c.notice("status_probe_failed", "status_probe "+p.Key+": probe dir missing")
		return Ready{}, false
	}
	if err := os.MkdirAll(c.Dir, 0o750); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+": "+err.Error())
		return Ready{}, false
	}
	scriptPath := filepath.Join(c.Dir, p.Key+".sh")
	metaPath := filepath.Join(c.Dir, p.Key+".meta.json")
	sum := intentHash(p.Intent, p.Path)
	if cachedOK(scriptPath, metaPath, sum) {
		return Ready{Key: p.Key, Command: []string{scriptPath}, Interval: interval, Timeout: timeout}, true
	}
	if !c.AIEnabled || c.Provider == nil {
		if probe.CheckScript(scriptPath) == nil {
			return Ready{Key: p.Key, Command: []string{scriptPath}, Interval: interval, Timeout: timeout}, true
		}
		c.notice("status_probe_skipped", "status_probe "+p.Key+": ai disabled and no compiled script")
		return Ready{}, false
	}
	body, err := c.generate(ctx, p)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	if err := validateGenerated(body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" compile: "+err.Error())
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	tmpPath := scriptPath + ".new"
	if err := writeScript(tmpPath, body); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" write: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	out, _, err := probe.RunScript(ctx, []string{tmpPath}, timeout, 0)
	if err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	if _, err := probe.ParseJSON(out); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" trial json: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	if err := os.Rename(tmpPath, scriptPath); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" install: "+err.Error())
		_ = os.Remove(tmpPath)
		return reuseOld(p.Key, scriptPath, interval, timeout)
	}
	_ = os.Chmod(scriptPath, scriptMode)
	if err := writeMeta(metaPath, probeMeta{Hash: sum, Intent: p.Intent, Path: p.Path}); err != nil {
		c.notice("status_probe_failed", "status_probe "+p.Key+" meta: "+err.Error())
	}
	return Ready{Key: p.Key, Command: []string{scriptPath}, Interval: interval, Timeout: timeout}, true
}

func reuseOld(key, scriptPath string, interval, timeout time.Duration) (Ready, bool) {
	if probe.CheckScript(scriptPath) == nil {
		return Ready{Key: key, Command: []string{scriptPath}, Interval: interval, Timeout: timeout}, true
	}
	return Ready{}, false
}

func (c *Compiler) generate(ctx context.Context, p config.StatusProbe) (string, error) {
	untrusted := "key=" + p.Key + "\nintent=" + p.Intent
	if p.Path != "" {
		untrusted += "\npath=" + p.Path
	}
	res, err := c.Provider.Run(ctx, aiprovider.Request{
		Task:      "probe_script",
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

func (c *Compiler) notice(code, md string) {
	if c.Notice != nil {
		c.Notice(code, md)
	}
}

func intentHash(intent, path string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(intent) + "\x00" + strings.TrimSpace(path)))
	return hex.EncodeToString(sum[:])
}

func cachedOK(scriptPath, metaPath, wantHash string) bool {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var m probeMeta
	if json.Unmarshal(b, &m) != nil || m.Hash != wantHash {
		return false
	}
	return probe.CheckScript(scriptPath) == nil
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
	for _, bad := range []string{"curl ", "wget ", "abp_m_", "agentboard_token", "/ingest/"} {
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
