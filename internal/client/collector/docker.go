package collector

import (
	"strings"

	"agentboard/internal/client/hostsnap"
)

// ReadDocker returns container and image inventory. Missing docker is not an error.
func ReadDocker(run Commander) *hostsnap.Docker {
	if run == nil {
		run = DefaultCommander
	}
	psOut, err := run("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Ports}}")
	if err != nil {
		return &hostsnap.Docker{Available: false}
	}
	info := &hostsnap.Docker{Available: true, Containers: ParseDockerPS(string(psOut))}
	imgOut, err := run("docker", "images", "-q")
	if err == nil {
		info.ImageCount = countNonEmptyLines(string(imgOut))
	}
	return info
}

// ParseDockerPS parses `docker ps -a --format` tab-separated rows.
func ParseDockerPS(output string) []hostsnap.Container {
	var out []hostsnap.Container
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		c := hostsnap.Container{
			ID:    strings.TrimSpace(parts[0]),
			Name:  strings.TrimSpace(parts[1]),
			Image: strings.TrimSpace(parts[2]),
			State: strings.ToLower(strings.TrimSpace(parts[3])),
		}
		if len(parts) >= 5 {
			c.Ports = strings.TrimSpace(parts[4])
		}
		if c.ID == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func countNonEmptyLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
