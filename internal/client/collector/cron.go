package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentboard/internal/client/hostsnap"
)

// CronTail remembers which execution keys have already been reported.
type CronTail struct {
	Seen map[string]bool `json:"seen"`
}

// ReadCronJobs reads enabled crontab lines from allowlisted paths under root.
// root is typically "" (real host) or a test fixture directory.
func ReadCronJobs(root string, extra []string) []hostsnap.CronJob {
	paths := []string{"/etc/crontab"}
	paths = append(paths, extra...)
	var jobs []hostsnap.CronJob
	seen := map[string]struct{}{}
	for _, p := range expandCronPaths(root, paths) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		system := isSystemCrontab(p)
		for _, j := range ParseCrontab(string(data), system) {
			key := j.Schedule + "\t" + j.User + "\t" + j.Command
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			jobs = append(jobs, j)
		}
	}
	return jobs
}

func expandCronPaths(root string, bases []string) []string {
	var out []string
	for _, b := range bases {
		p := joinRoot(root, b)
		out = append(out, p)
	}
	for _, dir := range []string{"/etc/cron.d", "/var/spool/cron/crontabs"} {
		entries, err := os.ReadDir(joinRoot(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".disabled") {
				continue
			}
			out = append(out, joinRoot(root, filepath.Join(dir, name)))
		}
	}
	return out
}

func joinRoot(root, p string) string {
	if root == "" {
		return p
	}
	return filepath.Join(root, p)
}

func isSystemCrontab(path string) bool {
	base := filepath.Base(path)
	if base == "crontab" && strings.Contains(path, "/etc/") {
		return true
	}
	return strings.Contains(path, "/cron.d/")
}

var cronMacros = map[string]bool{
	"@reboot": true, "@hourly": true, "@daily": true, "@weekly": true,
	"@monthly": true, "@yearly": true, "@annually": true, "@midnight": true,
}

// ParseCrontab parses crontab text. system files include a user column.
func ParseCrontab(text string, system bool) []hostsnap.CronJob {
	var jobs []hostsnap.CronJob
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") && !strings.ContainsAny(line[:strings.Index(line, "=")], " \t") {
			continue // env assignment
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var schedule, user, command string
		if cronMacros[fields[0]] {
			schedule = fields[0]
			rest := fields[1:]
			if system {
				if len(rest) < 2 {
					continue
				}
				user, command = rest[0], strings.Join(rest[1:], " ")
			} else {
				user, command = "", strings.Join(rest, " ")
			}
		} else {
			if len(fields) < 6 {
				continue
			}
			schedule = strings.Join(fields[0:5], " ")
			rest := fields[5:]
			if system {
				if len(rest) < 2 {
					continue
				}
				user, command = rest[0], strings.Join(rest[1:], " ")
			} else {
				user, command = "", strings.Join(rest, " ")
			}
		}
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		jobs = append(jobs, hostsnap.CronJob{Schedule: schedule, User: user, Command: command})
	}
	return jobs
}

// ParseCronLog extracts new CRON CMD lines from syslog/cron log text.
func ParseCronLog(text string, seen map[string]bool) []hostsnap.CronExec {
	if seen == nil {
		seen = map[string]bool{}
	}
	var out []hostsnap.CronExec
	for _, line := range strings.Split(text, "\n") {
		user, cmd, ok := parseCronLogLine(line)
		if !ok {
			continue
		}
		key := cronExecKey(line, user, cmd)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hostsnap.CronExec{
			Key:      key,
			Occurred: inferCronLogTime(line),
			User:     user,
			Command:  cmd,
		})
	}
	return out
}

func parseCronLogLine(line string) (user, cmd string, ok bool) {
	// (root) CMD (command)
	i := strings.Index(line, ") CMD (")
	if i < 0 {
		i = strings.Index(line, ") cmd (")
	}
	if i < 0 {
		return "", "", false
	}
	left := line[:i]
	lp := strings.LastIndex(left, "(")
	if lp < 0 {
		return "", "", false
	}
	user = strings.TrimSpace(left[lp+1:])
	rest := line[i+len(") CMD ("):]
	if strings.HasSuffix(rest, ")") {
		rest = rest[:len(rest)-1]
	}
	cmd = strings.TrimSpace(rest)
	if cmd == "" {
		return "", "", false
	}
	return user, cmd, true
}

func cronExecKey(line, user, cmd string) string {
	sum := sha256.Sum256([]byte(line + "\n" + user + "\n" + cmd))
	return hex.EncodeToString(sum[:12])
}

func inferCronLogTime(line string) string {
	// journalctl short-iso: 2026-08-26T03:20:01+08:00 hostname CRON...
	fields := strings.Fields(line)
	if len(fields) > 0 {
		if _, err := time.Parse(time.RFC3339, fields[0]); err == nil {
			return fields[0]
		}
	}
	return ""
}

// ReadCronExecutions reads incremental cron CMD lines from allowlisted log files
// and optional journalctl output.
func ReadCronExecutions(root string, logPaths []string, run Commander, tail *CronTail) []hostsnap.CronExec {
	if tail.Seen == nil {
		tail.Seen = map[string]bool{}
	}
	var all []hostsnap.CronExec
	if len(logPaths) == 0 {
		logPaths = []string{"/var/log/cron", "/var/log/cron.log"}
	}
	for _, p := range logPaths {
		data, err := os.ReadFile(joinRoot(root, p))
		if err != nil {
			continue
		}
		all = append(all, ParseCronLog(string(data), tail.Seen)...)
	}
	if run != nil {
		out, err := run("journalctl", "-n", "80", "--no-pager", "-o", "short-iso", "SYSLOG_IDENTIFIER=CRON")
		if err == nil {
			all = append(all, ParseCronLog(string(out), tail.Seen)...)
		}
	}
	return all
}
