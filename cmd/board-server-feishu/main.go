// Command board-server-feishu is the Feishu multi-user edition of board-server.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agentboard/internal/auth"
	"agentboard/internal/config"
	"agentboard/internal/feishu"
	"agentboard/internal/server"
	"agentboard/internal/store"
	"agentboard/internal/workspace"
	webui "agentboard/web"
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
		if err := runServer(); err != nil {
			fmt.Fprintf(os.Stderr, "board-server-feishu: %v\n", err)
			os.Exit(1)
		}
	case "version", "-version", "--version":
		fmt.Printf("board-server-feishu %s (%s) %s\n", version, commit, buildTime)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `board-server-feishu %s

Usage:
  board-server-feishu run
  board-server-feishu version

Feishu edition of AgentBoard. Required env: ABP_FEISHU_APP_ID, ABP_FEISHU_APP_SECRET.
Optional: ABP_FEISHU_TENANT_KEY, ABP_FEISHU_REDIRECT_URI (defaults to $ABP_PUBLIC_URL/auth/feishu/callback).
`, version)
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.ValidateFeishu(); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	if cfg.ClientUpdateDir != "" {
		if err := os.MkdirAll(cfg.ClientUpdateDir, 0o755); err != nil {
			return err
		}
	}

	secretKey, err := auth.LoadOrCreateSecretKey(cfg.DataDir, cfg.SecretKeyEnv)
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}

	log := newLogger(cfg.LogLevel)
	ctrl, err := workspace.OpenControl(filepath.Join(cfg.DataDir, "control.db"))
	if err != nil {
		return fmt.Errorf("control db: %w", err)
	}
	hub := workspace.NewHub(cfg.DataDir, ctrl, log)
	defer hub.Close()

	oauth := &feishu.OAuth{
		AppID:        cfg.FeishuAppID,
		AppSecret:    cfg.FeishuAppSecret,
		RedirectURI:  cfg.FeishuRedirectURI,
		APIBase:      cfg.FeishuAPIBase,
		AuthorizeURL: cfg.FeishuAuthorizeURL,
	}

	var webFS fs.FS
	if sub, err := webui.FS(); err != nil {
		log.Warn("embedded frontend unavailable", "err", err)
	} else if _, err := fs.Stat(sub, "index.html"); err != nil {
		log.Warn("frontend not built; API-only mode")
	} else {
		webFS = sub
	}

	s := server.NewFeishu(cfg, hub, oauth, log, webFS, secretKey)
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		runRetention := func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = hub.Control().DeleteExpiredSessions(c)
			policy := store.RetentionPolicy{
				EventDays:  cfg.EventRetention,
				MetricDays: cfg.RawMetricRetention,
				AccessDays: cfg.AccessRetention,
				QuotaBytes: cfg.EventQuotaBytes,
			}
			_ = hub.ForEachStore(c, func(slug string, st *store.Store) error {
				res, err := st.ApplyRetention(c, policy)
				if err != nil {
					log.Warn("workspace retention failed", "slug", slug, "err", err)
					return nil
				}
				if res.EventsDeleted+res.AccessDeleted+res.QuotaDeleted+res.ExpiredSessions+res.RunsClosed+res.RunsDeleted > 0 {
					log.Info("workspace retention",
						"slug", slug,
						"sessions", res.ExpiredSessions,
						"events", res.EventsDeleted,
						"access", res.AccessDeleted,
						"runs_closed", res.RunsClosed,
					)
				}
				return nil
			})
		}
		runRetention()
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		agentTick := time.NewTicker(2 * time.Minute)
		defer agentTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runRetention()
			case <-agentTick.C:
				c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_ = hub.ForEachStore(c, func(slug string, st *store.Store) error {
					n, err := st.CloseStaleAgentRuns(c)
					if err != nil {
						log.Warn("close stale agent runs failed", "slug", slug, "err", err)
					} else if n > 0 {
						log.Info("closed stale agent runs", "slug", slug, "runs_closed", n)
					}
					return nil
				})
				cancel()
			}
		}
	}()

	go s.RunClientUpdateSync(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("board-server-feishu listening",
			"addr", cfg.ListenAddr,
			"public_url", cfg.PublicURL,
			"edition", "feishu",
			"version", version,
			"commit", commit,
		)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
