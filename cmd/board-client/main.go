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

	"agentboard/internal/client/config"
	"agentboard/internal/client/runner"
	"agentboard/internal/client/spool"
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
		cfgPath := ""
		args := os.Args[2:]
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--config", "-c":
				if i+1 >= len(args) {
					fmt.Fprintln(os.Stderr, "--config requires a path")
					os.Exit(2)
				}
				i++
				cfgPath = args[i]
			case "-h", "--help":
				usage()
				os.Exit(0)
			default:
				fmt.Fprintf(os.Stderr, "unknown flag %q\n", args[i])
				usage()
				os.Exit(2)
			}
		}
		if cfgPath == "" {
			fmt.Fprintln(os.Stderr, "board-client run requires --config")
			usage()
			os.Exit(2)
		}
		if err := runClient(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "board-client: %v\n", err)
			os.Exit(1)
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
  board-client print-example-config
  board-client version

The machine token is read from the environment variable named in the config
(default ABP_MACHINE_TOKEN), never from the YAML file.
`, version)
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("board-client starting",
		"machine", cfg.Machine.Key,
		"server", cfg.Server.URL,
		"version", version,
		"commit", commit,
		"build_time", buildTime,
	)
	r.Run(ctx)
	log.Info("board-client stopped")
	return nil
}

// Keep in sync with configs/client.example.yaml.
const exampleConfig = `version: 1

server:
  url: "http://127.0.0.1:8080"
  # The machine token is read from this environment variable, never from the file.
  machine_token_env: "ABP_MACHINE_TOKEN"
  timeout: 20s
  tls_insecure_skip_verify: false

machine:
  key: "home-server"
  display_name: "家庭服务器"

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
`
