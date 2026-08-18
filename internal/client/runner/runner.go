// Package runner orchestrates the board-client collection and send loops.
package runner

import (
	"context"
	"encoding/json"
	"log/slog"
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
)

const selfServiceKey = "board-client"

// Runner wires collectors, spool and sender together.
type Runner struct {
	cfg *config.Config
	sp  *spool.Spool
	col *collector.Collector
	snd *sender.Sender
	log *slog.Logger

	bootID string
	seq    int64
}

// New constructs a Runner.
func New(cfg *config.Config, sp *spool.Spool, log *slog.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		sp:     sp,
		col:    collector.New(),
		snd:    sender.New(cfg.Server.URL, cfg.Token(), cfg.Server.Timeout.Duration),
		log:    log,
		bootID: shared.NewID(),
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
}

func (r *Runner) emitHeartbeat() {
	r.enqueue(event.TypeHeartbeat, "", "", event.Heartbeat{
		Hostname:                 r.cfg.Machine.DisplayName,
		OS:                       "linux",
		Arch:                     "amd64",
		CollectorVersion:         "1.0.0",
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
	logT := time.NewTicker(time.Minute)
	defer metricT.Stop()
	defer hbT.Stop()
	defer portsT.Stop()
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
		case <-logT.C:
			r.emitSelfLog("采集心跳正常，最近一分钟无异常。", "info")
		}
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
