// Command board-server is the AgentBoard Personal HTTP server and admin CLI.
package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"agentboard/internal/auth"
	"agentboard/internal/config"
	"agentboard/internal/server"
	"agentboard/internal/store"
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
			fmt.Fprintf(os.Stderr, "board-server: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err := runAdmin(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "board-server admin: %v\n", err)
			os.Exit(1)
		}
	case "version", "-version", "--version":
		fmt.Printf("board-server %s (%s) %s\n", version, commit, buildTime)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `board-server %s

Usage:
  board-server run
  board-server admin set-password [--password-stdin]
  board-server version

Configuration is read from ABP_* environment variables.
`, version)
}

func runAdmin(args []string) error {
	if len(args) == 0 || args[0] != "set-password" {
		return fmt.Errorf("usage: board-server admin set-password [--password-stdin]")
	}
	stdin := false
	for _, a := range args[1:] {
		switch a {
		case "--password-stdin":
			stdin = true
		case "-h", "--help":
			fmt.Fprintln(os.Stderr, "usage: board-server admin set-password [--password-stdin]")
			return nil
		default:
			return fmt.Errorf("unknown flag %q", a)
		}
	}
	pw, err := readPassword(stdin)
	if err != nil {
		return err
	}
	if pw == "" {
		return fmt.Errorf("password must not be empty")
	}
	if len(pw) < 12 {
		return fmt.Errorf("password must be at least 12 characters")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	st, err := openMigrated(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := st.SetAdminPassword(ctx, hash); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "admin password updated")
	return nil
}

func readPassword(fromStdin bool) (string, error) {
	if fromStdin || !term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	fmt.Fprint(os.Stderr, "New admin password: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.ArtifactDir, 0o750); err != nil {
		return err
	}

	secretKey, err := auth.LoadOrCreateSecretKey(cfg.DataDir, cfg.SecretKeyEnv)
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}

	log := newLogger(cfg.LogLevel)
	st, err := openMigrated(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	var webFS fs.FS
	if sub, err := webui.FS(); err != nil {
		log.Warn("embedded frontend unavailable", "err", err)
	} else if _, err := fs.Stat(sub, "index.html"); err != nil {
		log.Warn("frontend not built; API-only mode")
	} else {
		webFS = sub
	}

	s := server.New(cfg, st, log, webFS, secretKey)
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		runRetention := func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			res, err := st.ApplyRetention(c, store.RetentionPolicy{
				EventDays:  cfg.EventRetention,
				MetricDays: cfg.RawMetricRetention,
				AccessDays: cfg.AccessRetention,
				QuotaBytes: cfg.EventQuotaBytes,
			})
			cancel()
			if err != nil {
				log.Warn("retention cleanup failed", "err", err)
				return
			}
			if res.EventsDeleted+res.AccessDeleted+res.QuotaDeleted+res.ExpiredSessions > 0 {
				log.Info("retention cleanup",
					"sessions", res.ExpiredSessions,
					"events", res.EventsDeleted,
					"access", res.AccessDeleted,
					"quota", res.QuotaDeleted,
					"events_bytes", res.EventsBytes,
				)
			}
		}
		runRetention()
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runRetention()
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Info("board-server listening",
			"addr", cfg.ListenAddr,
			"public_url", cfg.PublicURL,
			"version", version,
			"commit", commit,
		)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func openMigrated(dbPath string) (*store.Store, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return st, nil
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
