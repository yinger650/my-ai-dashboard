// Package config loads and validates the board-client YAML configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
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
		Heartbeat   Duration `yaml:"heartbeat"`
		Metrics     Duration `yaml:"metrics"`
		Ports       Duration `yaml:"ports"`
		Systemd     Duration `yaml:"systemd"`
		CursorAgent Duration `yaml:"cursor_agent"`
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
			Enabled bool `yaml:"enabled"`
		} `yaml:"ports"`
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
	} `yaml:"collectors"`
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
	if c.Intervals.Heartbeat.Duration == 0 {
		c.Intervals.Heartbeat.Duration = 30 * time.Second
	}
	if c.Intervals.Metrics.Duration == 0 {
		c.Intervals.Metrics.Duration = 30 * time.Second
	}
	if c.Intervals.Ports.Duration == 0 {
		c.Intervals.Ports.Duration = time.Hour
	}
	if c.Intervals.Systemd.Duration == 0 {
		c.Intervals.Systemd.Duration = time.Minute
	}
	if c.Intervals.CursorAgent.Duration == 0 {
		c.Intervals.CursorAgent.Duration = 5 * time.Minute
	}
	if len(c.Collectors.Systemd.Include) == 0 {
		c.Collectors.Systemd.Include = []string{
			"board-server.service",
			"board-client.service",
			"nginx.service",
			"sshd.service",
		}
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
	return nil
}

// Token returns the machine token from the configured environment variable.
func (c *Config) Token() string { return os.Getenv(c.Server.MachineTokenEnv) }
