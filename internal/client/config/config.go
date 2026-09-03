// Package config loads and validates the board-client YAML configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
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

// MarshalYAML writes durations as strings like "30s".
func (d Duration) MarshalYAML() (any, error) {
	if d.Duration == 0 {
		return "0s", nil
	}
	return d.Duration.String(), nil
}

// Config is the client configuration (a practical subset of spec 14.2).
type Config struct {
	Version int `yaml:"version"`
	Server  struct {
		URL             string   `yaml:"url"`
		MachineToken    string   `yaml:"machine_token,omitempty"`
		MachineTokenEnv string   `yaml:"machine_token_env"`
		Timeout         Duration `yaml:"timeout"`
		TLSInsecure     bool     `yaml:"tls_insecure_skip_verify"`
	} `yaml:"server"`
	Machine struct {
		Key          string        `yaml:"key"`
		DisplayName  string        `yaml:"display_name"`
		StatusProbes []StatusProbe `yaml:"status_probes,omitempty"`
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
		Probe       Duration `yaml:"probe"`
		AISummary   Duration `yaml:"ai_summary"`
		AIDiscover  Duration `yaml:"ai_discover"`
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
		HTTP   HTTPCollector   `yaml:"http"`
		Probes ProbesCollector `yaml:"probes"`
	} `yaml:"collectors"`
	LocalIngest struct {
		Enabled       *bool  `yaml:"enabled"`
		Listen        string `yaml:"listen"`
		AdvertisePath string `yaml:"advertise_path"`
	} `yaml:"local_ingest"`
	AI     AIConfig     `yaml:"ai"`
	Update UpdateConfig `yaml:"update"`
}

// UpdateConfig pulls a newer board-client from a GitHub Release.
type UpdateConfig struct {
	Enabled  bool     `yaml:"enabled"`
	URL      string   `yaml:"url"`
	Interval Duration `yaml:"interval"`
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

// ProbesCollector runs user YAML-declared scripts.
type ProbesCollector struct {
	Enabled bool          `yaml:"enabled"`
	Scripts []ProbeScript `yaml:"scripts"`
}

// StatusProbe is a machine-level extra (GPU, directory usage, …) compiled
// into a side-path script. Distinct from collectors.probes virtual services.
type StatusProbe struct {
	Key      string   `yaml:"key"`
	Intent   string   `yaml:"intent"`
	Path     string   `yaml:"path,omitempty"`
	Command  []string `yaml:"command,omitempty"`
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
}

// ProbeScript is one local probe.
type ProbeScript struct {
	ServiceKey string   `yaml:"service_key"`
	Name       string   `yaml:"name"`
	Command    []string `yaml:"command"`
	Interval   Duration `yaml:"interval"`
	Timeout    Duration `yaml:"timeout"`
	Format     string   `yaml:"format"`
	TTLSeconds int      `yaml:"ttl_seconds"`
	MaxBytes   int      `yaml:"max_bytes"`
	AppendLog  *bool    `yaml:"append_log"`
}

// AIConfig is the local coding-agent CLI integration (spec 14.9).
type AIConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Provider          string        `yaml:"provider"`
	APIKeyEnv         string        `yaml:"api_key_env"`
	Workspace         string        `yaml:"workspace"`
	Command           []string      `yaml:"command"`
	Model             string        `yaml:"model"`
	Timeout           Duration      `yaml:"timeout"`
	MaxCallsPerDay    int           `yaml:"max_calls_per_day"`
	MaxInputBytes     int           `yaml:"max_input_bytes"`
	MaxOutputRunes    int           `yaml:"max_output_runes"`
	FallbackHeuristic *bool         `yaml:"fallback_heuristic"`
	Summarize         []AISummarize `yaml:"summarize"`
	Discover          AIDiscover    `yaml:"discover"`
}

// AISummarize is one log-summary source.
type AISummarize struct {
	Source     string   `yaml:"source"`
	ServiceKey string   `yaml:"service_key"`
	Name       string   `yaml:"name"`
	Interval   Duration `yaml:"interval"`
	MinNewLogs int      `yaml:"min_new_logs"`
	Prompt     string   `yaml:"prompt"`
}

// AIDiscover is the two-round host inspection.
type AIDiscover struct {
	Enabled           bool       `yaml:"enabled"`
	ServiceKey        string     `yaml:"service_key"`
	Name              string     `yaml:"name"`
	Interval          Duration   `yaml:"interval"`
	TTLSeconds        int        `yaml:"ttl_seconds"`
	MaxInvestigations int        `yaml:"max_investigations"`
	Prompt            string     `yaml:"prompt"`
	AllowCommands     []AllowCmd `yaml:"allow_commands"`
}

// AllowCmd is a whitelist command template for AI inspect round 2.
type AllowCmd struct {
	ID         string   `yaml:"id"`
	Argv       []string `yaml:"argv"`
	AllowPaths []string `yaml:"allow_paths,omitempty"`
}

var machineKeyRe = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// Read unmarshals a config file and applies defaults without validating the token.
func Read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, nil
}

// Load reads and validates a config file, applying defaults.
func Load(path string) (*Config, error) {
	c, err := Read(path)
	if err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
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
	if c.Intervals.Probe.Duration == 0 {
		c.Intervals.Probe.Duration = time.Minute
	}
	if c.Intervals.AISummary.Duration == 0 {
		c.Intervals.AISummary.Duration = 15 * time.Minute
	}
	if c.Intervals.AIDiscover.Duration == 0 {
		c.Intervals.AIDiscover.Duration = 6 * time.Hour
	}
	if c.AI.Provider == "" {
		c.AI.Provider = "cursor-agent"
	}
	if c.AI.APIKeyEnv == "" {
		c.AI.APIKeyEnv = "CURSOR_API_KEY"
	}
	if c.AI.Workspace == "" {
		dir := "/var/lib/agentboard-client/ai-workspace"
		if c.Storage.SpoolPath != "" {
			dir = filepath.Join(filepath.Dir(c.Storage.SpoolPath), "ai-workspace")
		}
		c.AI.Workspace = dir
	}
	if c.AI.Timeout.Duration == 0 {
		c.AI.Timeout.Duration = 120 * time.Second
	}
	if c.AI.MaxCallsPerDay == 0 {
		c.AI.MaxCallsPerDay = 48
	}
	if c.AI.MaxInputBytes == 0 {
		c.AI.MaxInputBytes = 32 * 1024
	}
	if c.AI.MaxOutputRunes == 0 {
		c.AI.MaxOutputRunes = 3000
	}
	if c.AI.Discover.ServiceKey == "" {
		c.AI.Discover.ServiceKey = "ai-inspect"
	}
	if c.AI.Discover.Name == "" {
		c.AI.Discover.Name = "AI 主机巡检"
	}
	if c.AI.Discover.Interval.Duration == 0 {
		c.AI.Discover.Interval = c.Intervals.AIDiscover
	}
	if c.AI.Discover.TTLSeconds == 0 {
		c.AI.Discover.TTLSeconds = 43200
	}
	if c.AI.Discover.MaxInvestigations == 0 {
		c.AI.Discover.MaxInvestigations = 8
	}
	for i := range c.AI.Summarize {
		s := &c.AI.Summarize[i]
		if s.Source == "" {
			s.Source = "agent_logs"
		}
		if s.ServiceKey == "" {
			s.ServiceKey = "ai-agent-digest"
		}
		if s.Name == "" {
			s.Name = "Agent 日志总结"
		}
		if s.Interval.Duration == 0 {
			s.Interval = c.Intervals.AISummary
		}
		if s.MinNewLogs == 0 {
			s.MinNewLogs = 3
		}
	}
	for i := range c.Machine.StatusProbes {
		s := &c.Machine.StatusProbes[i]
		if s.Interval.Duration == 0 {
			s.Interval = c.Intervals.Probe
		}
		if s.Timeout.Duration == 0 {
			s.Timeout.Duration = 15 * time.Second
		}
	}
	for i := range c.Collectors.Probes.Scripts {
		s := &c.Collectors.Probes.Scripts[i]
		if s.Format == "" {
			s.Format = "json"
		}
		if s.Interval.Duration == 0 {
			s.Interval = c.Intervals.Probe
		}
		if s.Timeout.Duration == 0 {
			s.Timeout.Duration = 15 * time.Second
		}
		if s.TTLSeconds == 0 {
			s.TTLSeconds = 180
		}
		if s.Name == "" {
			s.Name = s.ServiceKey
		}
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
	if c.LocalIngest.Listen == "" {
		c.LocalIngest.Listen = "127.0.0.1:7438"
	}
	if c.LocalIngest.AdvertisePath == "" {
		dir := "/var/lib/agentboard-client"
		if c.Storage.SpoolPath != "" {
			dir = filepath.Dir(c.Storage.SpoolPath)
		}
		c.LocalIngest.AdvertisePath = filepath.Join(dir, "local-ingest.json")
	}
	if c.Update.URL == "" {
		c.Update.URL = "https://github.com/yinger650/my-ai-dashboard/releases/latest/download"
	}
	if c.Update.Interval.Duration == 0 {
		c.Update.Interval.Duration = time.Hour
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
	if strings.TrimSpace(c.Token()) == "" {
		return fmt.Errorf("machine token missing: set %s or server.machine_token", c.Server.MachineTokenEnv)
	}
	if c.LocalIngestOn() {
		host, _, err := net.SplitHostPort(c.LocalIngest.Listen)
		if err != nil {
			return fmt.Errorf("local_ingest.listen: %w", err)
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("local_ingest.listen must be loopback, got %q", c.LocalIngest.Listen)
		}
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
	if c.AI.Enabled {
		switch strings.ToLower(c.AI.Provider) {
		case "cursor-agent", "cursor", "codex", "command":
		default:
			return fmt.Errorf("ai.provider must be cursor-agent, codex or command")
		}
		if strings.EqualFold(c.AI.Provider, "command") && len(c.AI.Command) == 0 {
			return fmt.Errorf("ai.command is required when provider=command")
		}
		if c.AI.Workspace != "" && !filepath.IsAbs(c.AI.Workspace) {
			return fmt.Errorf("ai.workspace must be an absolute path")
		}
		seen := map[string]struct{}{}
		for i, s := range c.AI.Summarize {
			if !machineKeyRe.MatchString(s.ServiceKey) {
				return fmt.Errorf("ai.summarize[%d].service_key must match [a-z0-9._-]{1,64}", i)
			}
			if _, dup := seen[s.ServiceKey]; dup {
				return fmt.Errorf("ai.summarize[%d].service_key %q is duplicated", i, s.ServiceKey)
			}
			seen[s.ServiceKey] = struct{}{}
			switch {
			case s.Source == "agent_logs", s.Source == "cursor_transcript", strings.HasPrefix(s.Source, "probe:"):
			default:
				return fmt.Errorf("ai.summarize[%d].source %q is not supported", i, s.Source)
			}
		}
		if c.AI.Discover.Enabled {
			if !machineKeyRe.MatchString(c.AI.Discover.ServiceKey) {
				return fmt.Errorf("ai.discover.service_key must match [a-z0-9._-]{1,64}")
			}
			ids := map[string]struct{}{}
			for i, cmd := range c.AI.Discover.AllowCommands {
				if cmd.ID == "" {
					return fmt.Errorf("ai.discover.allow_commands[%d].id is required", i)
				}
				if len(cmd.Argv) == 0 {
					return fmt.Errorf("ai.discover.allow_commands[%d].argv is required", i)
				}
				if _, dup := ids[cmd.ID]; dup {
					return fmt.Errorf("ai.discover.allow_commands[%d].id %q is duplicated", i, cmd.ID)
				}
				ids[cmd.ID] = struct{}{}
				usesPath := false
				for _, a := range cmd.Argv {
					if strings.Contains(a, "{path}") {
						usesPath = true
					}
				}
				if usesPath && len(cmd.AllowPaths) == 0 {
					return fmt.Errorf("ai.discover.allow_commands[%d].allow_paths is required for {path}", i)
				}
			}
		}
	}
	if c.Update.Enabled {
		u, err := url.Parse(c.Update.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("update.url must be an http(s) URL")
		}
	}
	if err := c.validateStatusProbes(); err != nil {
		return err
	}
	if c.Collectors.Probes.Enabled {
		seen := map[string]struct{}{}
		for i, s := range c.Collectors.Probes.Scripts {
			if !machineKeyRe.MatchString(s.ServiceKey) {
				return fmt.Errorf("collectors.probes.scripts[%d].service_key must match [a-z0-9._-]{1,64}", i)
			}
			if _, dup := seen[s.ServiceKey]; dup {
				return fmt.Errorf("collectors.probes.scripts[%d].service_key %q is duplicated", i, s.ServiceKey)
			}
			seen[s.ServiceKey] = struct{}{}
			if len(s.Command) == 0 {
				return fmt.Errorf("collectors.probes.scripts[%d].command is required", i)
			}
			if !filepath.IsAbs(s.Command[0]) {
				return fmt.Errorf("collectors.probes.scripts[%d].command[0] must be an absolute path", i)
			}
			switch strings.ToLower(s.Format) {
			case "json", "text":
			default:
				return fmt.Errorf("collectors.probes.scripts[%d].format must be json or text", i)
			}
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

const placeholderToken = "abp_m_REPLACE_ME"

func tokenValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == placeholderToken {
		return ""
	}
	return s
}

// Token returns the env token when set, otherwise server.machine_token.
func (c *Config) Token() string {
	if v := tokenValue(os.Getenv(c.Server.MachineTokenEnv)); v != "" {
		return v
	}
	return tokenValue(c.Server.MachineToken)
}

// ControlSockPath is the loopback unix socket next to the spool.
func (c *Config) ControlSockPath() string {
	return filepath.Join(filepath.Dir(c.Storage.SpoolPath), "control.sock")
}

// ProbeDir is where compiled status_probe scripts live.
func (c *Config) ProbeDir() string {
	return filepath.Join(filepath.Dir(c.Storage.SpoolPath), "probes")
}

func (c *Config) validateStatusProbes() error {
	seen := map[string]struct{}{}
	for i, s := range c.Machine.StatusProbes {
		if !machineKeyRe.MatchString(s.Key) {
			return fmt.Errorf("machine.status_probes[%d].key must match [a-z0-9._-]{1,64}", i)
		}
		if _, dup := seen[s.Key]; dup {
			return fmt.Errorf("machine.status_probes[%d].key %q is duplicated", i, s.Key)
		}
		seen[s.Key] = struct{}{}
		if strings.TrimSpace(s.Intent) == "" && len(s.Command) == 0 {
			return fmt.Errorf("machine.status_probes[%d] needs intent or command", i)
		}
		if len(s.Command) > 0 && !filepath.IsAbs(s.Command[0]) {
			return fmt.Errorf("machine.status_probes[%d].command[0] must be an absolute path", i)
		}
		if s.Path != "" && !filepath.IsAbs(s.Path) {
			return fmt.Errorf("machine.status_probes[%d].path must be absolute", i)
		}
	}
	return nil
}

// AtomicWrite marshals c to path via rename.
func AtomicWrite(path string, c *Config) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".client.yaml.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// LocalIngestOn is true unless explicitly disabled. Default on so coding agents can discover the client.
func (c *Config) LocalIngestOn() bool {
	if c.LocalIngest.Enabled == nil {
		return true
	}
	return *c.LocalIngest.Enabled
}

// FallbackHeuristicOn is true unless explicitly disabled.
func (c AIConfig) FallbackHeuristicOn() bool {
	if c.FallbackHeuristic == nil {
		return true
	}
	return *c.FallbackHeuristic
}
