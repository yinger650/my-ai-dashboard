package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"agentboard/internal/client/hostsnap"
)

// ProcExe returns the executable path for pid via /proc/<pid>/exe.
func ProcExe(pid int) string {
	if pid <= 0 {
		return ""
	}
	target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(target, " (deleted)")
}

// ParseExecStartPath extracts the binary from a systemd ExecStart= value.
func ParseExecStartPath(execStart string) string {
	execStart = strings.TrimSpace(execStart)
	if execStart == "" {
		return ""
	}
	if i := strings.Index(execStart, "path="); i >= 0 {
		rest := execStart[i+len("path="):]
		if j := strings.IndexAny(rest, " ;"); j >= 0 {
			rest = rest[:j]
		}
		return strings.TrimSpace(rest)
	}
	fields := strings.Fields(execStart)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "/") {
		return fields[0]
	}
	return ""
}

func parseMainPID(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func resolveUnitPath(u Unit) string {
	if u.MainPID > 0 {
		if p := ProcExe(u.MainPID); p != "" {
			return p
		}
	}
	if p := ParseExecStartPath(u.ExecStart); p != "" {
		return p
	}
	return ""
}

func annotateProcessPaths(snap *hostsnap.Snapshot) {
	if snap == nil {
		return
	}
	for i := range snap.Ports {
		if snap.Ports[i].PID > 0 {
			snap.Ports[i].Exe = ProcExe(snap.Ports[i].PID)
		}
	}
	if snap.Nginx != nil {
		if snap.Nginx.PID > 0 {
			snap.Nginx.Exe = ProcExe(snap.Nginx.PID)
		}
		if snap.Nginx.Exe == "" {
			snap.Nginx.Exe = unitPath(snap.Units, "nginx")
		}
	}
	if snap.Docker != nil {
		snap.Docker.Exe = firstPortExe(snap.Ports, "dockerd", "docker")
		if snap.Docker.Exe == "" {
			snap.Docker.Exe = unitPath(snap.Units, "docker")
		}
	}
	if snap.Cron != nil {
		snap.Cron.Exe = unitPath(snap.Units, "cron", "crond", "cronie")
	}
}

func firstPortExe(ports []hostsnap.Port, names ...string) string {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	for _, p := range ports {
		base := p.Process
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if !want[base] && !want[p.Process] {
			continue
		}
		if p.Exe != "" {
			return p.Exe
		}
		if strings.HasPrefix(p.Process, "/") {
			return p.Process
		}
	}
	return ""
}

func unitPath(units []hostsnap.Unit, names ...string) string {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
		want[n+".service"] = true
	}
	for _, u := range units {
		if u.Path == "" {
			continue
		}
		key := strings.TrimSuffix(u.Unit, ".service")
		if want[u.Unit] || want[key] {
			return u.Path
		}
	}
	return ""
}
