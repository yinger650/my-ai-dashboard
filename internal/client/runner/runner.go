// Package runner orchestrates the board-client collection and send loops.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/sender"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/summarize"
)

const selfServiceKey = "board-client"

// Runner wires collectors, spool and sender together.
type Runner struct {
	cfg *config.Config
	sp  *spool.Spool
	col *collector.Collector
	snd *sender.Sender
	log *slog.Logger

	bootID   string
	seq      int64
	seenPath string
}

// New constructs a Runner.
func New(cfg *config.Config, sp *spool.Spool, log *slog.Logger) *Runner {
	return &Runner{
		cfg:      cfg,
		sp:       sp,
		col:      collector.New(),
		snd:      sender.New(cfg.Server.URL, cfg.Token(), cfg.Server.Timeout.Duration),
		log:      log,
		bootID:   shared.NewID(),
		seenPath: filepath.Join(filepath.Dir(cfg.Storage.SpoolPath), "cursor-seen.json"),
	}
}

func (r *Runner) enqueue(eventType, serviceKey, runKey string, payload any) {
	seq := atomic.AddInt64(&r.seq, 1)
	pb, _ := json.Marshal(payload)
	env := event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     eventType,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		BootID:        r.bootID,
		Sequence:      &seq,
		ServiceKey:    serviceKey,
		RunKey:        runKey,
		Payload:       pb,
	}
	raw, _ := json.Marshal(env)
	if err := r.sp.Enqueue(env.EventID, eventType, string(raw)); err != nil {
		r.log.Warn("spool enqueue failed", "err", err)
	}
}

// CollectOnce gathers one round of heartbeat, metric, ports and self-status.
func (r *Runner) CollectOnce() {
	r.emitHeartbeat()
	r.emitMetric()
	r.emitPorts()
	r.emitSelfState()
	r.emitSelfStatus()
	r.emitSystemd()
	r.emitCursorAgent()
}

func (r *Runner) emitHeartbeat() {
	r.enqueue(event.TypeHeartbeat, "", "", event.Heartbeat{
		Hostname:                 r.cfg.Machine.DisplayName,
		OS:                       "linux",
		Arch:                     "amd64",
		CollectorVersion:         "1.1.0",
		HeartbeatIntervalSeconds: int(r.cfg.Intervals.Heartbeat.Duration.Seconds()),
		UptimeSeconds:            r.col.Uptime(),
	})
}

func (r *Runner) emitMetric() {
	c := r.cfg.Collectors
	ms := r.col.Sample(
		c.Filesystems.Enabled, c.Filesystems.IncludeMounts, c.Filesystems.ExcludeFSType, c.Network.ExcludeInterfaces,
		c.CPU, c.Memory, c.DiskIO.Enabled, c.Network.Enabled,
	)
	r.enqueue(event.TypeMetricSample, "", "", ms)
}

func (r *Runner) emitPorts() {
	if !r.cfg.Collectors.Ports.Enabled {
		return
	}
	ports, ok := collector.ReadPorts()
	if !ok {
		return
	}
	r.enqueue(event.TypePortSnapshot, "", "", map[string]any{"ports": ports})
}

func (r *Runner) emitSelfState() {
	r.enqueue(event.TypeServiceState, selfServiceKey, "", event.ServiceState{
		Name:     "Board Client",
		Type:     "daemon",
		State:    "running",
		Summary:  "collecting metrics",
		Severity: "normal",
	})
}

func (r *Runner) emitSelfStatus() {
	n, _ := r.sp.Count()
	r.enqueue(event.TypeStatusUpsert, selfServiceKey, "", event.StatusUpsert{
		Items: []event.StatusItem{
			{Key: "uptime", Label: "系统运行时间", Value: json.RawMessage(itoa(r.col.Uptime())), ValueType: "duration", Unit: "s", Severity: "normal", DisplayFormat: "duration", SortOrder: 10},
			{Key: "spool_queue", Label: "待发送队列", Value: json.RawMessage(itoa(int64(n))), ValueType: "number", Unit: "", Severity: sevForQueue(n), DisplayFormat: "number", SortOrder: 20},
		},
	})
}

func (r *Runner) emitSelfLog(markdown, severity string) {
	r.enqueue(event.TypeLogAppend, selfServiceKey, "", event.LogPayload{Markdown: markdown, Severity: severity, Source: "board-client"})
}

// Run starts all loops and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	// Startup ping (non-fatal unless auth error).
	if retriable, err := r.snd.Ping(ctx); err != nil {
		if !retriable {
			r.log.Error("auth failed on ping; check machine token", "err", err)
			return
		}
		r.log.Warn("ping failed; buffering offline", "err", err)
	} else {
		r.log.Info("connected to board-server", "url", r.cfg.Server.URL)
	}

	r.CollectOnce()
	r.emitSelfLog("board-client 启动，开始采集系统指标。", "info")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); r.senderLoop(ctx) }()

	metricT := time.NewTicker(r.cfg.Intervals.Metrics.Duration)
	hbT := time.NewTicker(r.cfg.Intervals.Heartbeat.Duration)
	portsT := time.NewTicker(r.cfg.Intervals.Ports.Duration)
	sysT := time.NewTicker(r.cfg.Intervals.Systemd.Duration)
	curT := time.NewTicker(r.cfg.Intervals.CursorAgent.Duration)
	logT := time.NewTicker(time.Minute)
	defer metricT.Stop()
	defer hbT.Stop()
	defer portsT.Stop()
	defer sysT.Stop()
	defer curT.Stop()
	defer logT.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-hbT.C:
			r.emitHeartbeat()
		case <-metricT.C:
			r.emitMetric()
			r.emitSelfStatus()
		case <-portsT.C:
			r.emitPorts()
		case <-sysT.C:
			r.emitSystemd()
		case <-curT.C:
			r.emitCursorAgent()
		case <-logT.C:
			r.emitSelfLog("采集心跳正常，最近一分钟无异常。", "info")
		}
	}
}

func (r *Runner) emitSystemd() {
	cfg := r.cfg.Collectors.Systemd
	if !cfg.Enabled {
		return
	}
	units, err := collector.ReadSystemdUnits(cfg.IncludeAll, cfg.Include)
	if err != nil {
		r.enqueue(event.TypeCollectorNotice, selfServiceKey, "", event.CollectorNotice{
			Severity: "warning",
			Code:     "systemd_read_failed",
			Markdown: "读取 systemd unit 失败：" + err.Error(),
		})
		return
	}
	filtered := collector.FilterUnits(units, cfg.IncludeAll, cfg.Include, cfg.ExcludePrefixes)
	if len(filtered) == 0 {
		return
	}
	r.enqueue(event.TypeServiceSnapshot, "", "", collector.UnitsToSnapshot(filtered))
}

type cursorSeenFile struct {
	Size    int64 `json:"size"`
	ModUnix int64 `json:"mtime"`
}

type cursorSeenState struct {
	Files map[string]cursorSeenFile `json:"files"`
}

func (r *Runner) loadCursorSeen() cursorSeenState {
	st := cursorSeenState{Files: map[string]cursorSeenFile{}}
	b, err := os.ReadFile(r.seenPath)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	if st.Files == nil {
		st.Files = map[string]cursorSeenFile{}
	}
	return st
}

func (r *Runner) saveCursorSeen(st cursorSeenState) {
	b, _ := json.Marshal(st)
	_ = os.MkdirAll(filepath.Dir(r.seenPath), 0o750)
	_ = os.WriteFile(r.seenPath, b, 0o600)
}

func (r *Runner) emitCursorAgent() {
	cfg := r.cfg.Collectors.CursorAgent
	if !cfg.Enabled {
		return
	}
	files := collector.ScanTranscripts(cfg.Paths)
	n := len(files)
	state, sev, summary := "idle", "unknown", "未发现 transcript"
	if n > 0 {
		state, sev, summary = "running", "normal", "已扫描 "+strconv.Itoa(n)+" 个 transcript"
	}
	r.enqueue(event.TypeServiceState, cfg.ServiceKey, "", event.ServiceState{
		Name:     cfg.ServiceName,
		Type:     "agent",
		State:    state,
		Summary:  summary,
		Severity: sev,
	})
	if n == 0 {
		return
	}
	seen := r.loadCursorSeen()
	changed := false
	var bodies []string
	for _, f := range files {
		prev, ok := seen.Files[f.Path]
		if ok && prev.Size == f.Size && prev.ModUnix == f.ModUnix {
			continue
		}
		changed = true
		seen.Files[f.Path] = cursorSeenFile{Size: f.Size, ModUnix: f.ModUnix}

		sum := sha256.Sum256([]byte(f.Path))
		runKey := hex.EncodeToString(sum[:6]) + "-" + strconv.FormatInt(f.ModUnix, 10)
		started := time.Unix(f.ModUnix, 0).UTC()
		finished := shared.FormatTime(started)
		r.enqueue(event.TypeRunTransition, cfg.ServiceKey, runKey, event.RunTransition{
			ServiceName: cfg.ServiceName,
			ServiceType: "agent",
			Status:      "succeeded",
			Summary:     f.Title,
			StartedAt:   finished,
			FinishedAt:  finished,
			Provider:    "cursor",
			Metadata:    map[string]any{"path": f.Rel, "size": f.Size},
		})
		r.enqueue(event.TypeLogAppend, cfg.ServiceKey, runKey, event.LogPayload{
			Markdown: "### " + f.Title + "\n\n" + f.Text,
			Severity: "info",
			Source:   "cursor-agent",
		})
		bodies = append(bodies, f.Text)
	}
	if !changed {
		return
	}
	r.saveCursorSeen(seen)
	if cfg.PinSummary && len(bodies) > 0 {
		md := summarize.Logs(cfg.ServiceName+" 日志总结", bodies)
		r.enqueue(event.TypeLogPin, cfg.ServiceKey, "", event.LogPayload{
			Markdown: md,
			Severity: "info",
			Source:   "cursor-agent",
		})
	}
}

func (r *Runner) senderLoop(ctx context.Context) {
	failures := 0
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// Final flush attempt.
			r.drainOnce(ctx)
			return
		case <-t.C:
			if dropped, _ := r.sp.Trim(r.cfg.Storage.MaxEvents); dropped > 0 {
				r.log.Warn("spool trimmed", "dropped", dropped)
			}
			ok := r.drainOnce(ctx)
			if ok {
				failures = 0
			} else {
				failures++
			}
		}
	}
}

// DrainAll repeatedly sends batches until the spool is empty or ctx is done.
func (r *Runner) DrainAll(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		n, _ := r.sp.Count()
		if n == 0 {
			return
		}
		if !r.drainOnce(ctx) {
			return
		}
	}
}

// drainOnce sends one batch. Returns false on transient failure.
func (r *Runner) drainOnce(ctx context.Context) bool {
	batch, err := r.sp.Batch(100, 256*1024)
	if err != nil {
		r.log.Warn("spool batch failed", "err", err)
		return false
	}
	if len(batch) == 0 {
		return true
	}
	res := r.snd.Send(ctx, batch)
	if res.Err != nil {
		ids := make([]string, len(batch))
		for i, b := range batch {
			ids[i] = b.EventID
		}
		_ = r.sp.MarkRetry(ids, sender.Backoff(1), res.Err.Error())
		r.log.Warn("send failed; will retry", "err", res.Err, "count", len(ids))
		return false
	}
	if err := r.sp.Delete(res.DeleteIDs); err != nil {
		r.log.Warn("spool delete failed", "err", err)
	}
	r.log.Debug("batch sent", "delivered", len(res.DeleteIDs))
	return true
}

func itoa(v int64) []byte {
	return []byte(strconv.FormatInt(v, 10))
}

func sevForQueue(n int) string {
	if n > 10000 {
		return "warning"
	}
	return "normal"
}
