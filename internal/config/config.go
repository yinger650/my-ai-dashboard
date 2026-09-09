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
	EventQuotaBytes    int64
	SessionHours       int
	TrustedProxyCIDRs  []string
	LogLevel           string
	SecureCookies      bool
	SecretKeyEnv       string
	ClientUpdateDir    string
	ClientUpdateToken  string
	ClientUpdateSource string
	ClientUpdateSync   bool

	// Feishu edition (board-server-feishu). Empty in personal edition.
	FeishuAppID        string
	FeishuAppSecret    string
	FeishuRedirectURI  string
	FeishuTenantKey    string
	FeishuAPIBase      string
	FeishuAuthorizeURL string
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
		ListenAddr:         getenv("ABP_LISTEN_ADDR", "127.0.0.1:8080"),
		PublicURL:          os.Getenv("ABP_PUBLIC_URL"),
		DataDir:            dataDir,
		DBPath:             getenv("ABP_DB_PATH", filepath.Join(dataDir, "board.db")),
		ArtifactDir:        getenv("ABP_ARTIFACT_DIR", filepath.Join(dataDir, "artifacts")),
		LogLevel:           getenv("ABP_LOG_LEVEL", "info"),
		SecretKeyEnv:       os.Getenv("ABP_SECRET_KEY"),
		ClientUpdateToken:  os.Getenv("ABP_CLIENT_UPDATE_TOKEN"),
		ClientUpdateSource: getenv("ABP_CLIENT_UPDATE_SOURCE", "https://github.com/yinger650/my-ai-dashboard/releases/latest/download"),
		FeishuAppID:        os.Getenv("ABP_FEISHU_APP_ID"),
		FeishuAppSecret:    os.Getenv("ABP_FEISHU_APP_SECRET"),
		FeishuRedirectURI:  os.Getenv("ABP_FEISHU_REDIRECT_URI"),
		FeishuTenantKey:    os.Getenv("ABP_FEISHU_TENANT_KEY"),
		FeishuAPIBase:      getenv("ABP_FEISHU_API_BASE", "https://open.feishu.cn"),
		FeishuAuthorizeURL: getenv("ABP_FEISHU_AUTHORIZE_URL", "https://accounts.feishu.cn/open-apis/authen/v1/authorize"),
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
	if c.EventRetention, err = getInt("ABP_EVENT_RETENTION_DAYS", 30); err != nil {
		return nil, err
	}
	if c.AccessRetention, err = getInt("ABP_ACCESS_RETENTION_DAYS", 30); err != nil {
		return nil, err
	}
	if c.EventQuotaBytes, err = getInt64("ABP_EVENT_QUOTA_BYTES", 5*1024*1024*1024); err != nil {
		return nil, err
	}
	if c.SessionHours, err = getInt("ABP_SESSION_HOURS", 12); err != nil {
		return nil, err
	}

	c.SecureCookies = getBool("ABP_SECURE_COOKIES", true)
	c.ClientUpdateSync = getBool("ABP_CLIENT_UPDATE_SYNC", true)
	c.ClientUpdateDir = getenv("ABP_CLIENT_UPDATE_DIR", filepath.Join(dataDir, "client-updates"))

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
	if c.FeishuRedirectURI == "" && c.PublicURL != "" {
		c.FeishuRedirectURI = strings.TrimRight(c.PublicURL, "/") + "/auth/feishu/callback"
	}

	return c, nil
}

// ValidateFeishu reports whether Feishu edition required fields are set.
func (c *Config) ValidateFeishu() error {
	if strings.TrimSpace(c.FeishuAppID) == "" {
		return fmt.Errorf("ABP_FEISHU_APP_ID is required")
	}
	if strings.TrimSpace(c.FeishuAppSecret) == "" {
		return fmt.Errorf("ABP_FEISHU_APP_SECRET is required")
	}
	return nil
}
