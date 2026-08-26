package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentboard/internal/client/hostsnap"
)

const (
	// CronLookback is the max age of a cron execution we will report.
	CronLookback = 15 * time.Minute
	// CronMaxPerRound caps Run/log events per collect cycle.
	CronMaxPerRound = 15
	// CronTailBytes is how far back we read a log the first time we see it.
	CronTailBytes = 64 * 1024
	// CronSeenMax is when we drop the in-memory dedup set (offsets still stand).
	CronSeenMax = 4000
)

// CronTail remembers which execution keys have already been reported and the
// byte offset of each cron log we have already consumed.
type CronTail struct {
	Seen    map[string]bool  `json:"seen"`
	Offsets map[string]int64 `json:"offsets"`
	Primed  bool             `json:"primed"`
}

func (t *CronTail) ensure() {
	if t.Seen == nil {
		t.Seen = map[string]bool{}
	}
	if t.Offsets == nil {
		t.Offsets = map[string]int64{}
	}
}

func (t *CronTail) pruneSeen() {
	if len(t.Seen) < CronSeenMax {
		return
	}
	t.Seen = map[string]bool{}
}

// IsCronNoise reports cloud-agent / vendor housekeeping jobs that should not
// appear on the board (Tencent Cloud stargate, TAT, etc.).
func IsCronNoise(command string) bool {
	c := strings.ToLower(command)
	for _, n := range []string{
		"/usr/local/qcloud/",
		"/usr/local/qcloud",
		"stargate",
		"tat_agent",
		"tat-agent",
		"yunjing",
		"barad_agent",
		"barad",
	} {
		if strings.Contains(c, n) {
			return true
		}
	}
	return false
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
			if IsCronNoise(j.Command) {
				continue
			}
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
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, fields[0]); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02T15:04:05-0700", fields[0]); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if len(fields) >= 3 {
		raw := strings.Join(fields[0:3], " ")
		year := time.Now().Year()
		if t, err := time.ParseInLocation("2006 Jan 2 15:04:05", fmt.Sprintf("%d %s", year, raw), time.Local); err == nil {
			if t.After(time.Now().Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func keepCronExec(ex hostsnap.CronExec, primed bool, cutoff time.Time) bool {
	if IsCronNoise(ex.Command) {
		return false
	}
	if ex.Occurred == "" {
		// Untimed lines are only safe after we have started tailing.
		return primed
	}
	t, err := time.Parse(time.RFC3339, ex.Occurred)
	if err != nil {
		return primed
	}
	return !t.Before(cutoff)
}

func readCronFileDelta(path string, tail *CronTail) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	size := info.Size()
	start, known := tail.Offsets[path]
	if !known {
		start = size - CronTailBytes
		if start < 0 {
			start = 0
		}
	} else if start > size {
		start = 0 // rotated
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	tail.Offsets[path] = size
	return string(data)
}

// ReadCronExecutions reads incremental cron CMD lines from allowlisted log files
// and optional journalctl output. It never replays a whole historical log.
func ReadCronExecutions(root string, logPaths []string, run Commander, tail *CronTail) []hostsnap.CronExec {
	if tail == nil {
		tail = &CronTail{}
	}
	tail.ensure()
	tail.pruneSeen()

	primed := tail.Primed
	cutoff := time.Now().Add(-CronLookback)
	var all []hostsnap.CronExec
	if len(logPaths) == 0 {
		logPaths = []string{"/var/log/cron", "/var/log/cron.log"}
	}
	gotFile := false
	for _, p := range logPaths {
		full := joinRoot(root, p)
		delta := readCronFileDelta(full, tail)
		if delta == "" {
			continue
		}
		gotFile = true
		all = append(all, ParseCronLog(delta, tail.Seen)...)
	}
	if !gotFile && run != nil {
		since := cutoff.Format(time.RFC3339)
		out, err := run("journalctl", "-n", "40", "--no-pager", "-o", "short-iso", "--since", since, "SYSLOG_IDENTIFIER=CRON")
		if err == nil {
			all = append(all, ParseCronLog(string(out), tail.Seen)...)
		}
	}

	var kept []hostsnap.CronExec
	for _, ex := range all {
		if !keepCronExec(ex, primed, cutoff) {
			continue
		}
		kept = append(kept, ex)
		if len(kept) >= CronMaxPerRound {
			break
		}
	}
	tail.Primed = true
	return kept
}
