// Command board-client collects host metrics and reports them to board-server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"agentboard/internal/client/cfgui"
	"agentboard/internal/client/config"
	"agentboard/internal/client/runner"
	"agentboard/internal/client/spool"
	"agentboard/internal/client/update"
	"agentboard/internal/client/wrap"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		cfgPath, err := requireConfig(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			usage()
			os.Exit(2)
		}
		if err := runClient(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "board-client: %v\n", err)
			os.Exit(1)
		}
	case "wrap":
		opt, argv, err := wrap.ParseArgs(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		code, err := wrap.Run(opt, argv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "board-client wrap: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
		os.Exit(code)
	case "config":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "tui":
			cfgPath, err := requireConfig(os.Args[3:])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			if err := cfgui.RunTUI(cfgPath, os.Stdin, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "board-client config tui: %v\n", err)
				os.Exit(1)
			}
		case "web":
			cfgPath, listen, err := parseWebArgs(os.Args[3:])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			if err := cfgui.RunWeb(ctx, cfgPath, listen); err != nil {
				fmt.Fprintf(os.Stderr, "board-client config web: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown config command %q\n", os.Args[2])
			usage()
			os.Exit(2)
		}
	case "print-example-config":
		os.Stdout.WriteString(exampleConfig)
	case "version", "-version", "--version":
		fmt.Printf("board-client %s (%s) %s\n", version, commit, buildTime)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `board-client %s

Usage:
  board-client run --config client.yaml
  board-client wrap --summary TEXT [--ttl 6h] [--log PATH] [--config client.yaml] -- CMD...
  board-client config tui --config client.yaml
  board-client config web --config client.yaml [--listen 127.0.0.1:7439]
  board-client print-example-config
  board-client version

Token: non-empty $ABP_MACHINE_TOKEN overrides server.machine_token in YAML.
config tui/web inherit key and server.url, toggle built-in features, and keep custom lists.
status_probe scripts are compiled locally; the board never sends commands.
wrap and agentboard-report are mutually exclusive for the same task.
`, version)
}

func requireConfig(args []string) (string, error) {
	path, _, err := parseConfigFlags(args)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("--config is required")
	}
	return path, nil
}

func parseWebArgs(args []string) (cfgPath, listen string, err error) {
	listen = "127.0.0.1:7439"
	cfgPath, rest, err := parseConfigFlags(args)
	if err != nil {
		return "", "", err
	}
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--listen":
			if i+1 >= len(rest) {
				return "", "", fmt.Errorf("--listen requires a value")
			}
			i++
			listen = rest[i]
		default:
			return "", "", fmt.Errorf("unknown flag %q", rest[i])
		}
	}
	if cfgPath == "" {
		return "", "", fmt.Errorf("--config is required")
	}
	return cfgPath, listen, nil
}

func parseConfigFlags(args []string) (cfgPath string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--config requires a path")
			}
			i++
			cfgPath = args[i]
		case "-h", "--help":
			return "", nil, fmt.Errorf("help")
		default:
			rest = append(rest, args[i])
		}
	}
	return cfgPath, rest, nil
}

func runClient(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.SpoolPath), 0o750); err != nil {
		return err
	}
	sp, err := spool.Open(cfg.Storage.SpoolPath)
	if err != nil {
		return err
	}
	defer sp.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	r := runner.New(cfg, sp, log)
	r.Build = update.Info{Version: version, Commit: commit}
	r.SetConfigPath(cfgPath)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("board-client starting",
		"machine", cfg.Machine.Key,
		"server", cfg.Server.URL,
		"version", version,
		"commit", commit,
		"build_time", buildTime,
		"auto_update", cfg.Update.Enabled,
	)
	r.Run(ctx)
	log.Info("board-client stopped")
	return nil
}

// Keep in sync with configs/client.example.yaml.
const exampleConfig = `version: 1

server:
  url: "http://127.0.0.1:8080"
  machine_token: "abp_m_REPLACE_ME"
  machine_token_env: "ABP_MACHINE_TOKEN"
  timeout: 20s
  tls_insecure_skip_verify: false

machine:
  key: "home-server"
  display_name: "家庭服务器"
  status_probes:
    - key: gpu
      intent: "NVIDIA GPU 利用率 0-100"
    - key: data_dir
      intent: "/data 占用百分比"
      path: /data
      interval: 60s

storage:
  spool_path: "/var/lib/agentboard-client/spool.db"
  max_events: 50000

intervals:
  collect: 60s
  heartbeat: 30s
  metrics: 30s
  ports: 60s
  systemd: 60s
  cursor_agent: 5m
  http: 60s

local_ingest:
  enabled: true
  listen: "127.0.0.1:7438"
  advertise_path: "/var/lib/agentboard-client/local-ingest.json"
  # Loopback copy of agent events. board-client projects them to proj-*
  # with this client's token and tees log.append for AI digest.

update:
  enabled: false
  # 国内机器请改成看板自己的镜像，避免直连 GitHub Release（会 302 到 Azure CDN）：
  # url: "http://127.0.0.1:8090/client-updates"
  # url: "https://board.yinger650.com/client-updates"
  url: "https://github.com/yinger650/my-ai-dashboard/releases/latest/download"
  interval: 1h

collectors:
  cpu: true
  memory: true
  filesystems:
    enabled: true
    include_mounts: ["/"]
    exclude_fs_types: ["tmpfs", "devtmpfs", "overlay", "squashfs"]
  disk_io:
    enabled: true
  network:
    enabled: true
    exclude_interfaces: ["lo", "veth*", "docker*"]
  ports:
    enabled: true
  docker:
    enabled: true
  cron:
    enabled: true
  nginx:
    enabled: true
    config_paths:
      - /etc/nginx
      - /www/server/nginx/conf
  systemd:
    enabled: false
    include_all: false
    include:
      - board-server.service
      - board-client.service
      - nginx.service
      - sshd.service
    exclude_prefixes:
      - systemd-
      - user@
      - getty@
      - session-
  cursor_agent:
    enabled: false
    service_key: cursor-agent
    service_name: Cursor Agent
    pin_summary: true
    paths:
      - /root/.cursor/projects
      - /root/.cursor-server
  http:
    enabled: false
    timeout: 10s
    follow_redirects: true
    warn_latency: 3s
    ttl_seconds: 180
    targets:
      - service_key: site-board
        name: AgentBoard
        url: "https://board.yinger650.com/health/live"
        method: GET
        expect_status: [200]
  probes:
    enabled: false
    scripts:
      - service_key: gpu
        name: GPU 节点
        command: ["/etc/agentboard/probes/gpu.sh"]
        interval: 60s
        timeout: 15s
        format: json
        ttl_seconds: 180

ai:
  enabled: false
  provider: cursor-agent
  api_key_env: "CURSOR_API_KEY"
  workspace: "/var/lib/agentboard-client/ai-workspace"
  timeout: 120s
  max_calls_per_day: 48
  fallback_heuristic: true
  summarize:
    - source: agent_logs
      service_key: ai-agent-digest
      name: Agent 日志总结
      interval: 15m
      min_new_logs: 3
  discover:
    enabled: false
    service_key: ai-inspect
    name: AI 主机巡检
    interval: 6h
    ttl_seconds: 43200
    max_investigations: 8
    allow_commands:
      - id: unit_status
        argv: ["systemctl", "status", "--no-pager", "-n", "50", "{unit}"]
      - id: unit_journal
        argv: ["journalctl", "--no-pager", "-n", "200", "-u", "{unit}"]
      - id: read_file
        argv: ["cat", "{path}"]
        allow_paths: ["/var/log/**", "/etc/agentboard/**"]
`
