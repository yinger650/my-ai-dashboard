// Package config loads board-server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the resolved server configuration.
type Config struct {
	ListenAddr         string
	PublicURL          string
	DataDir            string
	DBPath             string
	ArtifactDir        string
	MaxUploadBytes     int64
	ArtifactQuotaBytes int64
	RawMetricRetention int
	EventRetention     int
	AccessRetention    int
	SessionHours       int
	TrustedProxyCIDRs  []string
	LogLevel           string
	SecureCookies      bool
	SecretKeyEnv       string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

// Load reads configuration from the environment. DataDir defaults are applied
// so paths under it are derived unless explicitly overridden.
func Load() (*Config, error) {
	dataDir := getenv("ABP_DATA_DIR", "/var/lib/agentboard")

	c := &Config{
		ListenAddr:   getenv("ABP_LISTEN_ADDR", "127.0.0.1:8080"),
		PublicURL:    os.Getenv("ABP_PUBLIC_URL"),
		DataDir:      dataDir,
		DBPath:       getenv("ABP_DB_PATH", filepath.Join(dataDir, "board.db")),
		ArtifactDir:  getenv("ABP_ARTIFACT_DIR", filepath.Join(dataDir, "artifacts")),
		LogLevel:     getenv("ABP_LOG_LEVEL", "info"),
		SecretKeyEnv: os.Getenv("ABP_SECRET_KEY"),
	}

	var err error
	if c.MaxUploadBytes, err = getInt64("ABP_MAX_UPLOAD_BYTES", 10*1024*1024); err != nil {
		return nil, err
	}
	if c.ArtifactQuotaBytes, err = getInt64("ABP_ARTIFACT_QUOTA_BYTES", 5*1024*1024*1024); err != nil {
		return nil, err
	}
	if c.RawMetricRetention, err = getInt("ABP_RAW_METRIC_RETENTION_DAYS", 30); err != nil {
		return nil, err
	}
	if c.EventRetention, err = getInt("ABP_EVENT_RETENTION_DAYS", 90); err != nil {
		return nil, err
	}
	if c.AccessRetention, err = getInt("ABP_ACCESS_RETENTION_DAYS", 90); err != nil {
		return nil, err
	}
	if c.SessionHours, err = getInt("ABP_SESSION_HOURS", 12); err != nil {
		return nil, err
	}

	c.SecureCookies = getBool("ABP_SECURE_COOKIES", true)

	if cidrs := os.Getenv("ABP_TRUSTED_PROXY_CIDRS"); cidrs != "" {
		for _, p := range strings.Split(cidrs, ",") {
			if s := strings.TrimSpace(p); s != "" {
				c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, s)
			}
		}
	}

	if c.PublicURL == "" {
		// PublicURL is required in production, but default to the listen addr so
		// local/dev startup is not blocked. Callers may warn on this.
		c.PublicURL = "http://" + c.ListenAddr
	}

	return c, nil
}
