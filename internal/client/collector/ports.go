package collector

import (
	"strconv"
	"strings"

	"agentboard/internal/client/hostsnap"
)

// Port is an alias for the snapshot listening socket type.
type Port = hostsnap.Port

// ReadPorts returns listening ports via `ss -H -lntup`. It never uses a shell.
// If ss is unavailable it returns (nil, false).
func ReadPorts() ([]Port, bool) {
	return ReadPortsCmd(DefaultCommander)
}

// ReadPortsCmd is ReadPorts with an injectable commander.
func ReadPortsCmd(run Commander) ([]Port, bool) {
	if run == nil {
		run = DefaultCommander
	}
	out, err := run("ss", "-H", "-lntup")
	if err != nil {
		return nil, false
	}
	return ParseSS(string(out)), true
}

// ParseSS parses `ss -H -lntup` output.
func ParseSS(output string) []Port {
	var ports []Port
	for _, line := range strings.Split(output, "\n") {
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
	return ports
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
