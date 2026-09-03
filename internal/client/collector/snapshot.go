package collector

import (
	"os"
	"strconv"
	"strings"

	"agentboard/internal/client/config"
	"agentboard/internal/client/hostsnap"
)

// CollectOptions controls one Part 1 collect round.
type CollectOptions struct {
	HostRoot  string
	Run       Commander
	CronTail  *CronTail
	CronLogs  []string
	CronExtra []string
}

// Collect gathers a HostSnapshot. It never ingest. Missing optional tools
// leave that block unavailable instead of failing the round.
func (c *Collector) Collect(cfg *config.Config, opt CollectOptions) hostsnap.Snapshot {
	if opt.Run == nil {
		opt.Run = DefaultCommander
	}
	col := cfg.Collectors
	snap := hostsnap.Snapshot{
		UptimeSeconds: c.Uptime(),
		Metric: c.Sample(
			col.Filesystems.Enabled, col.Filesystems.IncludeMounts, col.Filesystems.ExcludeFSType, col.Network.ExcludeInterfaces,
			col.CPU, col.Memory, col.DiskIO.Enabled, col.Network.Enabled,
		),
	}
	if col.Ports.Enabled {
		if ports, ok := ReadPortsCmd(opt.Run); ok {
			snap.Ports = ports
		}
	}
	if col.Docker.Enabled {
		snap.Docker = ReadDocker(opt.Run)
	}
	if col.Cron.Enabled {
		jobs := ReadCronJobs(opt.HostRoot, opt.CronExtra)
		if opt.CronTail == nil {
			opt.CronTail = &CronTail{Seen: map[string]bool{}}
		}
		execs := ReadCronExecutions(opt.HostRoot, opt.CronLogs, opt.Run, opt.CronTail)
		snap.Cron = &hostsnap.Cron{Jobs: jobs, Executions: execs}
	}
	if col.Nginx.Enabled {
		paths := col.Nginx.ConfigPaths
		ngx := ReadNginx(paths, opt.HostRoot)
		ngx.PID = nginxPID(opt.HostRoot, snap.Ports)
		snap.Nginx = ngx
	}
	if col.Systemd.Enabled {
		units, err := ReadSystemdUnitsCmd(opt.Run, col.Systemd.IncludeAll, col.Systemd.Include)
		if err == nil {
			filtered := FilterUnits(units, col.Systemd.IncludeAll, col.Systemd.Include, col.Systemd.ExcludePrefixes)
			for _, u := range filtered {
				snap.Units = append(snap.Units, hostsnap.Unit{
					Unit: u.Unit, Load: u.Load, Active: u.Active, Sub: u.Sub, Description: u.Description, Path: u.Path,
				})
			}
		}
	}
	annotateProcessPaths(&snap)
	return snap
}

func nginxPID(root string, ports []hostsnap.Port) int {
	for _, p := range ports {
		if p.Process == "nginx" && p.PID > 0 {
			return p.PID
		}
	}
	for _, cand := range []string{"/run/nginx.pid", "/var/run/nginx.pid"} {
		b, err := os.ReadFile(joinRoot(root, cand))
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}
