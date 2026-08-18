package collector

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Port describes one listening socket.
type Port struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	PID      int    `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
}

// ReadPorts returns listening ports via `ss -H -lntup`. It never uses a shell.
// If ss is unavailable it returns (nil, false).
func ReadPorts() ([]Port, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ss", "-H", "-lntup")
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var ports []Port
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		proto := fields[0]
		local := fields[4]
		addr, portStr := splitHostPort(local)
		p, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}
		entry := Port{Protocol: proto, Address: addr, Port: p}
		if pid, proc, ok := parseProcess(line); ok {
			entry.PID, entry.Process = pid, proc
		}
		ports = append(ports, entry)
	}
	return ports, true
}

func splitHostPort(s string) (host, port string) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return s, ""
	}
	host = strings.Trim(s[:idx], "[]")
	return host, s[idx+1:]
}

// parseProcess extracts pid/process from the users:(("name",pid=123,fd=4)) column.
func parseProcess(line string) (int, string, bool) {
	i := strings.Index(line, `users:(("`)
	if i < 0 {
		return 0, "", false
	}
	rest := line[i+len(`users:(("`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return 0, "", false
	}
	name := rest[:end]
	pidIdx := strings.Index(rest, "pid=")
	if pidIdx < 0 {
		return 0, name, true
	}
	pidRest := rest[pidIdx+4:]
	comma := strings.IndexAny(pidRest, ",)")
	if comma < 0 {
		return 0, name, true
	}
	pid, _ := strconv.Atoi(pidRest[:comma])
	return pid, name, true
}
