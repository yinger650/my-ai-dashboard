package store

// Machine mirrors the machines table.
type Machine struct {
	ID                       string  `json:"id"`
	MachineKey               string  `json:"machine_key"`
	Name                     string  `json:"name"`
	Kind                     string  `json:"kind"`
	Description              string  `json:"description"`
	OS                       *string `json:"os"`
	Arch                     *string `json:"arch"`
	Hostname                 *string `json:"hostname"`
	CollectorVersion         *string `json:"collector_version"`
	BootID                   *string `json:"boot_id"`
	HeartbeatIntervalSeconds int     `json:"heartbeat_interval_seconds"`
	LastSeenAt               *string `json:"last_seen_at"`
	LastEventAt              *string `json:"last_event_at"`
	Enabled                  bool    `json:"enabled"`
	AutoCreateServices       bool    `json:"auto_create_services"`
	MetadataJSON             string  `json:"metadata_json"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
	DeletedAt                *string `json:"deleted_at,omitempty"`
}

// Service mirrors the services table.
type Service struct {
	ID           string  `json:"id"`
	MachineID    string  `json:"machine_id"`
	ServiceKey   string  `json:"service_key"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Description  string  `json:"description"`
	CurrentState string  `json:"current_state"`
	StateSummary string  `json:"state_summary"`
	Severity     string  `json:"severity"`
	TTLSeconds   *int    `json:"ttl_seconds"`
	LastSeenAt   *string `json:"last_seen_at"`
	LastRunAt    *string `json:"last_run_at"`
	Enabled      bool    `json:"enabled"`
	SortOrder    int     `json:"sort_order"`
	MetadataJSON string  `json:"metadata_json"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// Token mirrors the api_tokens table (never includes the secret).
type Token struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	TokenPrefix           string  `json:"token_prefix"`
	TokenHash             string  `json:"-"`
	Scope                 string  `json:"scope"`
	MachineID             *string `json:"machine_id"`
	ServiceID             *string `json:"service_id"`
	IPAllowlistJSON       string  `json:"ip_allowlist_json"`
	RequestsPerMinute     int     `json:"requests_per_minute"`
	BytesPerDay           int64   `json:"bytes_per_day"`
	AllowArtifactDownload bool    `json:"allow_artifact_download"`
	LastUsedAt            *string `json:"last_used_at"`
	LastUsedIP            *string `json:"last_used_ip"`
	ExpiresAt             *string `json:"expires_at"`
	Enabled               bool    `json:"enabled"`
	CreatedAt             string  `json:"created_at"`
	RevokedAt             *string `json:"revoked_at"`
}

// Run mirrors the runs table.
type Run struct {
	ID              string  `json:"id"`
	ServiceID       string  `json:"service_id"`
	RunKey          string  `json:"run_key"`
	Status          string  `json:"status"`
	Summary         string  `json:"summary"`
	StartedAt       *string `json:"started_at"`
	FinishedAt      *string `json:"finished_at"`
	Provider        *string `json:"provider"`
	ProviderAgentID *string `json:"provider_agent_id"`
	ProviderRunID   *string `json:"provider_run_id"`
	DurationMs      *int64  `json:"duration_ms"`
	InputTokens     *int64  `json:"input_tokens"`
	OutputTokens    *int64  `json:"output_tokens"`
	MetadataJSON    string  `json:"metadata_json"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ActiveRun is a non-terminal run shown on the board.
type ActiveRun struct {
	ID          string  `json:"id"`
	ServiceID   string  `json:"service_id"`
	ServiceKey  string  `json:"service_key"`
	ServiceName string  `json:"service_name"`
	RunKey      string  `json:"run_key"`
	Status      string  `json:"status"`
	Summary     string  `json:"summary"`
	StartedAt   *string `json:"started_at"`
	CreatedAt   string  `json:"created_at"`
}

// MetricSample mirrors metric_samples.
type MetricSample struct {
	OccurredAt         string   `json:"occurred_at"`
	CPUPercent         *float64 `json:"cpu_percent"`
	Load1              *float64 `json:"load1"`
	Load5              *float64 `json:"load5"`
	Load15             *float64 `json:"load15"`
	MemoryUsedBytes    *int64   `json:"memory_used_bytes"`
	MemoryTotalBytes   *int64   `json:"memory_total_bytes"`
	SwapUsedBytes      *int64   `json:"swap_used_bytes"`
	SwapTotalBytes     *int64   `json:"swap_total_bytes"`
	DiskReadBps        *float64 `json:"disk_read_bps"`
	DiskWriteBps       *float64 `json:"disk_write_bps"`
	NetworkRxBps       *float64 `json:"network_rx_bps"`
	NetworkTxBps       *float64 `json:"network_tx_bps"`
	RootDiskUsedBytes  *int64   `json:"root_disk_used_bytes"`
	RootDiskTotalBytes *int64   `json:"root_disk_total_bytes"`
	ExtraJSON          string   `json:"extra_json"`
}

// CurrentStatus mirrors current_status.
type CurrentStatus struct {
	ServiceID     string  `json:"service_id"`
	StatusKey     string  `json:"status_key"`
	Label         string  `json:"label"`
	ValueJSON     string  `json:"value_json"`
	ValueType     string  `json:"value_type"`
	Unit          *string `json:"unit"`
	Severity      string  `json:"severity"`
	DisplayFormat string  `json:"display_format"`
	SortOrder     int     `json:"sort_order"`
	OccurredAt    string  `json:"occurred_at"`
	UpdatedAt     string  `json:"updated_at"`
	ServiceKey    string  `json:"service_key,omitempty"`
	ServiceName   string  `json:"service_name,omitempty"`
}

// PinnedLog mirrors pinned_logs.
type PinnedLog struct {
	ServiceID   string `json:"service_id"`
	EventID     string `json:"event_id"`
	Markdown    string `json:"markdown"`
	Severity    string `json:"severity"`
	OccurredAt  string `json:"occurred_at"`
	UpdatedAt   string `json:"updated_at"`
	ServiceKey  string `json:"service_key,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}

// LogEntry is a projected view of a log.append event.
type LogEntry struct {
	EventID     string `json:"event_id"`
	Markdown    string `json:"markdown"`
	Severity    string `json:"severity"`
	Source      string `json:"source,omitempty"`
	OccurredAt  string `json:"occurred_at"`
	ServiceID   string `json:"service_id,omitempty"`
	ServiceKey  string `json:"service_key,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	RunKey      string `json:"run_key,omitempty"`
}

// AccessLog mirrors access_logs.
type AccessLog struct {
	ID         string  `json:"id"`
	OccurredAt string  `json:"occurred_at"`
	RequestID  string  `json:"request_id"`
	ActorType  string  `json:"actor_type"`
	ActorID    *string `json:"actor_id"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	StatusCode int     `json:"status_code"`
	IP         *string `json:"ip"`
	UserAgent  *string `json:"user_agent"`
	BytesIn    int64   `json:"bytes_in"`
	DurationMs int64   `json:"duration_ms"`
	Result     string  `json:"result"`
	Reason     string  `json:"reason"`
	IsAbnormal bool    `json:"is_abnormal"`
}

// AdminCredentials mirrors admin_credentials.
type AdminCredentials struct {
	PasswordHash          string
	TOTPSecretEncrypted   *string
	RecoveryCodesHashJSON *string
	FailedAttempts        int
	LockedUntil           *string
	UpdatedAt             string
}

// Artifact mirrors artifacts.
type Artifact struct {
	ID            string  `json:"id"`
	UploadEventID string  `json:"upload_event_id"`
	MachineID     string  `json:"machine_id"`
	ServiceID     *string `json:"service_id"`
	StoredName    string  `json:"stored_name"`
	OriginalName  string  `json:"original_name"`
	MIMEType      string  `json:"mime_type"`
	SizeBytes     int64   `json:"size_bytes"`
	SHA256        string  `json:"sha256"`
	CreatedAt     string  `json:"created_at"`
	DeletedAt     *string `json:"deleted_at,omitempty"`
}

// Session mirrors admin_sessions.
type Session struct {
	ID            string
	TokenHash     string
	CSRFTokenHash string
	CreatedAt     string
	ExpiresAt     string
	LastSeenAt    string
	IP            *string
	UserAgent     *string
}
