// Package hostsnap holds the Part 1 host fact snapshot. Collectors fill it;
// the Part 2 agent projects it into Board events. It is not an ingest payload.
package hostsnap

import "agentboard/internal/event"

// Snapshot is one Part 1 collect round.
type Snapshot struct {
	UptimeSeconds int64
	Metric        event.MetricSample
	Ports         []Port
	Docker        *Docker
	Cron          *Cron
	Nginx         *Nginx
	Units         []Unit
}

// Port is one listening socket.
type Port struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	PID      int    `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
	Exe      string `json:"exe,omitempty"`
}

// Docker is a compact docker inventory. Nil means the collector was disabled.
type Docker struct {
	Available  bool
	ImageCount int
	Containers []Container
	Exe        string
}

// Container is one docker ps -a row.
type Container struct {
	ID    string
	Name  string
	Image string
	State string
	Ports string
}

// Cron is crontab jobs plus newly observed executions.
type Cron struct {
	Jobs       []CronJob
	Executions []CronExec
	Exe        string
}

// CronJob is one enabled crontab line.
type CronJob struct {
	Schedule string
	User     string
	Command  string
}

// CronExec is one observed cron run from logs.
type CronExec struct {
	Key       string
	Occurred  string
	User      string
	Command   string
	Succeeded *bool
}

// Nginx is loaded reverse-proxy config plus process identity.
type Nginx struct {
	Available bool
	PID       int
	Reloads   int
	Proxies   []Proxy
	Exe       string
}

// Proxy is one location that reverse-proxies.
type Proxy struct {
	ServerName string
	Listen     string
	ListenPort int
	Location   string
	Upstream   string
}

// Unit is a systemd unit fact.
type Unit struct {
	Unit        string
	Load        string
	Active      string
	Sub         string
	Description string
	Path        string
}
