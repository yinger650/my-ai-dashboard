// Package config loads and validates the board-client YAML configuration.
package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration for YAML string parsing ("30s", "1h").
type Duration struct{ time.Duration }

// UnmarshalYAML parses a duration string.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	dur, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = dur
	return nil
}

// Config is the client configuration (a practical subset of spec 14.2).
type Config struct {
	Version int `yaml:"version"`
	Server  struct {
		URL             string   `yaml:"url"`
		MachineTokenEnv string   `yaml:"machine_token_env"`
		Timeout         Duration `yaml:"timeout"`
		TLSInsecure     bool     `yaml:"tls_insecure_skip_verify"`
	} `yaml:"server"`
	Machine struct {
		Key         string `yaml:"key"`
		DisplayName string `yaml:"display_name"`
	} `yaml:"machine"`
	Storage struct {
		SpoolPath string `yaml:"spool_path"`
		MaxEvents int    `yaml:"max_events"`
	} `yaml:"storage"`
	Intervals struct {
		Collect     Duration `yaml:"collect"`
		Heartbeat   Duration `yaml:"heartbeat"`
		Metrics     Duration `yaml:"metrics"`
		Ports       Duration `yaml:"ports"`
		Systemd     Duration `yaml:"systemd"`
		CursorAgent Duration `yaml:"cursor_agent"`
		HTTP        Duration `yaml:"http"`
	} `yaml:"intervals"`
	Collectors struct {
		CPU         bool `yaml:"cpu"`
		Memory      bool `yaml:"memory"`
		Filesystems struct {
			Enabled       bool     `yaml:"enabled"`
			IncludeMounts []string `yaml:"include_mounts"`
			ExcludeFSType []string `yaml:"exclude_fs_types"`
		} `yaml:"filesystems"`
		DiskIO struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"disk_io"`
		Network struct {
			Enabled           bool     `yaml:"enabled"`
			ExcludeInterfaces []string `yaml:"exclude_interfaces"`
		} `yaml:"network"`
		Ports struct {
			Enabled bool          `yaml:"enabled"`
			Promote []PromoteRule `yaml:"promote"`
		} `yaml:"ports"`
		Docker struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"docker"`
		Cron struct {
			Enabled  bool     `yaml:"enabled"`
			LogPaths []string `yaml:"log_paths"`
		} `yaml:"cron"`
		Nginx struct {
			Enabled     bool     `yaml:"enabled"`
			ConfigPaths []string `yaml:"config_paths"`
		} `yaml:"nginx"`
		Systemd struct {
			Enabled         bool     `yaml:"enabled"`
			IncludeAll      bool     `yaml:"include_all"`
			Include         []string `yaml:"include"`
			ExcludePrefixes []string `yaml:"exclude_prefixes"`
		} `yaml:"systemd"`
		CursorAgent struct {
			Enabled     bool     `yaml:"enabled"`
			ServiceKey  string   `yaml:"service_key"`
			ServiceName string   `yaml:"service_name"`
			PinSummary  bool     `yaml:"pin_summary"`
			Paths       []string `yaml:"paths"`
		} `yaml:"cursor_agent"`
		HTTP HTTPCollector `yaml:"http"`
	} `yaml:"collectors"`
}

// PromoteRule maps a listening process name to a Board daemon service.
type PromoteRule struct {
	Process    string `yaml:"process"`
	ServiceKey string `yaml:"service_key"`
	Name       string `yaml:"name"`
}

// HTTPCollector probes remote websites and reports each as a virtual service.
type HTTPCollector struct {
	Enabled         bool         `yaml:"enabled"`
	Timeout         Duration     `yaml:"timeout"`
	FollowRedirects *bool        `yaml:"follow_redirects"`
	WarnLatency     Duration     `yaml:"warn_latency"`
	TTLSeconds      int          `yaml:"ttl_seconds"`
	Targets         []HTTPTarget `yaml:"targets"`
}

// HTTPTarget is one website health check.
type HTTPTarget struct {
	ServiceKey     string            `yaml:"service_key"`
	Name           string            `yaml:"name"`
	URL            string            `yaml:"url"`
	Method         string            `yaml:"method"`
	ExpectStatus   []int             `yaml:"expect_status"`
	ExpectContains string            `yaml:"expect_contains"`
	Headers        map[string]string `yaml:"headers"`
	TLSInsecure    bool              `yaml:"tls_insecure_skip_verify"`
}

var machineKeyRe = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// Load reads and validates a config file, applying defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Server.MachineTokenEnv == "" {
		c.Server.MachineTokenEnv = "ABP_MACHINE_TOKEN"
	}
	if c.Server.Timeout.Duration == 0 {
		c.Server.Timeout.Duration = 20 * time.Second
	}
	if c.Storage.SpoolPath == "" {
		c.Storage.SpoolPath = "/var/lib/agentboard-client/spool.db"
	}
	if c.Storage.MaxEvents == 0 {
		c.Storage.MaxEvents = 50000
	}
	if c.Intervals.Collect.Duration == 0 {
		c.Intervals.Collect.Duration = time.Minute
	}
	if c.Intervals.Heartbeat.Duration == 0 {
		c.Intervals.Heartbeat.Duration = 30 * time.Second
	}
	if c.Intervals.Metrics.Duration == 0 {
		c.Intervals.Metrics.Duration = 30 * time.Second
	}
	if c.Intervals.Ports.Duration == 0 {
		c.Intervals.Ports.Duration = time.Minute
	}
	if c.Intervals.Systemd.Duration == 0 {
		c.Intervals.Systemd.Duration = time.Minute
	}
	if c.Intervals.CursorAgent.Duration == 0 {
		c.Intervals.CursorAgent.Duration = 5 * time.Minute
	}
	if c.Intervals.HTTP.Duration == 0 {
		c.Intervals.HTTP.Duration = time.Minute
	}
	if c.Collectors.HTTP.Timeout.Duration == 0 {
		c.Collectors.HTTP.Timeout.Duration = 10 * time.Second
	}
	if c.Collectors.HTTP.WarnLatency.Duration == 0 {
		c.Collectors.HTTP.WarnLatency.Duration = 3 * time.Second
	}
	if c.Collectors.HTTP.TTLSeconds == 0 {
		c.Collectors.HTTP.TTLSeconds = 180
	}
	for i := range c.Collectors.HTTP.Targets {
		t := &c.Collectors.HTTP.Targets[i]
		if t.Method == "" {
			t.Method = "GET"
		} else {
			t.Method = strings.ToUpper(t.Method)
		}
		if len(t.ExpectStatus) == 0 {
			t.ExpectStatus = []int{200}
		}
		u, err := url.Parse(t.URL)
		if err != nil {
			continue
		}
		if t.Name == "" {
			t.Name = u.Hostname()
		}
		if t.ServiceKey == "" && u.Hostname() != "" {
			t.ServiceKey = "site-" + strings.ReplaceAll(strings.ToLower(u.Hostname()), ".", "-")
		}
	}
	if len(c.Collectors.Systemd.Include) == 0 {
		c.Collectors.Systemd.Include = []string{
			"board-server.service",
			"board-client.service",
			"nginx.service",
			"sshd.service",
			"docker.service",
			"crond.service",
		}
	}
	if len(c.Collectors.Ports.Promote) == 0 {
		c.Collectors.Ports.Promote = []PromoteRule{
			{Process: "nginx", ServiceKey: "nginx", Name: "Nginx"},
			{Process: "sshd", ServiceKey: "sshd", Name: "sshd"},
			{Process: "board-server", ServiceKey: "board-server", Name: "Board Server"},
		}
	}
	if len(c.Collectors.Nginx.ConfigPaths) == 0 {
		c.Collectors.Nginx.ConfigPaths = []string{"/etc/nginx", "/www/server/nginx/conf"}
	}
	if len(c.Collectors.Systemd.ExcludePrefixes) == 0 {
		c.Collectors.Systemd.ExcludePrefixes = []string{"systemd-", "user@", "getty@", "session-"}
	}
	if c.Collectors.CursorAgent.ServiceKey == "" {
		c.Collectors.CursorAgent.ServiceKey = "cursor-agent"
	}
	if c.Collectors.CursorAgent.ServiceName == "" {
		c.Collectors.CursorAgent.ServiceName = "Cursor Agent"
	}
	if len(c.Collectors.CursorAgent.Paths) == 0 {
		c.Collectors.CursorAgent.Paths = []string{
			"/root/.cursor/projects",
			"/root/.cursor-server",
		}
	}
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}
	if !machineKeyRe.MatchString(c.Machine.Key) {
		return fmt.Errorf("machine.key must match [a-z0-9._-]{1,64}")
	}
	if os.Getenv(c.Server.MachineTokenEnv) == "" {
		return fmt.Errorf("environment variable %s (machine token) is empty", c.Server.MachineTokenEnv)
	}
	if c.Collectors.HTTP.Enabled {
		seen := map[string]struct{}{}
		for i, t := range c.Collectors.HTTP.Targets {
			if t.URL == "" {
				return fmt.Errorf("collectors.http.targets[%d].url is required", i)
			}
			u, err := url.Parse(t.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("collectors.http.targets[%d].url must be an http(s) URL", i)
			}
			if !machineKeyRe.MatchString(t.ServiceKey) {
				return fmt.Errorf("collectors.http.targets[%d].service_key must match [a-z0-9._-]{1,64}", i)
			}
			if _, dup := seen[t.ServiceKey]; dup {
				return fmt.Errorf("collectors.http.targets[%d].service_key %q is duplicated", i, t.ServiceKey)
			}
			seen[t.ServiceKey] = struct{}{}
		}
	}
	return nil
}

// HTTPFollowRedirects is true unless explicitly disabled.
func (c *Config) HTTPFollowRedirects() bool {
	if c.Collectors.HTTP.FollowRedirects == nil {
		return true
	}
	return *c.Collectors.HTTP.FollowRedirects
}

// Token returns the machine token from the configured environment variable.
func (c *Config) Token() string { return os.Getenv(c.Server.MachineTokenEnv) }
