// Package runner orchestrates the board-client collection and send loops.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"agentboard/internal/client/agent"
	"agentboard/internal/client/aiinspect"
	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/hostsnap"
	"agentboard/internal/client/localingest"
	"agentboard/internal/client/probe"
	"agentboard/internal/client/projreport"
	"agentboard/internal/client/sender"
	"agentboard/internal/client/spool"
	"agentboard/internal/client/statusprobe"
	"agentboard/internal/client/update"
	"agentboard/internal/client/wrapd"
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

	bootID    string
	seq       int64
	seenPath  string
	statePath string
	httpPrev  map[string]bool
	proj      *agent.State
	cronTail  *collector.CronTail
	lastSnap  *hostsnap.Snapshot
	hostRoot  string
	runCmd    collector.Commander

	mu        sync.Mutex
	logBuf    *aiinspect.Buffer
	provider  aiprovider.Provider
	lastProbe map[string]time.Time
	lastSum   map[string]time.Time
	lastDisc  time.Time
	probePin  map[string]string

	cfgPath       string
	wrap          *wrapd.Manager
	statusReady   map[string]*liveStatus
	hbMeta        map[string]any
	probeMetaKeys map[string][]string
	lastWrapSum   map[string]time.Time

	Build update.Info
}

type liveStatus struct {
	statusprobe.Ready
	fails int
}

// New constructs a Runner.
func New(cfg *config.Config, sp *spool.Spool, log *slog.Logger) *Runner {
	dir := filepath.Dir(cfg.Storage.SpoolPath)
	r := &Runner{
		cfg:           cfg,
		sp:            sp,
		col:           collector.New(),
		snd:           sender.New(cfg.Server.URL, cfg.Token(), cfg.Server.Timeout.Duration),
		log:           log,
		bootID:        shared.NewID(),
		seenPath:      filepath.Join(dir, "cursor-seen.json"),
		statePath:     filepath.Join(dir, "inspect-state.json"),
		httpPrev:      map[string]bool{},
		proj:          agent.NewState(),
		cronTail:      &collector.CronTail{Seen: map[string]bool{}},
		logBuf:        aiinspect.NewBuffer(sp),
		lastProbe:     map[string]time.Time{},
		lastSum:       map[string]time.Time{},
		probePin:      map[string]string{},
		statusReady:   map[string]*liveStatus{},
		hbMeta:        map[string]any{},
		probeMetaKeys: map[string][]string{},
		lastWrapSum:   map[string]time.Time{},
	}
	r.wrap = wrapd.New()
	r.wrap.Enqueue = r.enqueue
	r.wrap.Debug = func(msg string) {
		if r.log != nil {
			r.log.Debug(msg)
		}
	}
	r.wrap.Audit = r.noteTaskDone
	r.wrap.Summarize = r.summarizeWrap
	return r
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

func (r *Runner) ingestProjectCopy(env event.Envelope) {
	for _, p := range projreport.Project(env) {
		r.enqueue(p.Type, p.ServiceKey, p.RunKey, p.Payload)
		if env.EventType == event.TypeRunTransition {
			var rt event.RunTransition
			_ = json.Unmarshal(env.Payload, &rt)
			r.noteTaskDone(p.RunKey, p.ServiceKey, rt.Status, rt.Summary)
		}
	}
}

// CollectOnce runs Part 1 (facts) then Part 2 (agent project) and optional extras.
func (r *Runner) CollectOnce() {
	r.collectAndProject()
	r.emitCursorAgent()
	r.emitHTTP()
	r.emitProbes()
	r.emitStatusProbes()
	r.emitAISummaries()
	r.emitAIDiscover()
}

func (r *Runner) collectAndProject() {
	n, _ := r.sp.Count()
	opt := collector.CollectOptions{
		HostRoot: r.hostRoot,
		Run:      r.runCmd,
		CronTail: r.cronTail,
		CronLogs: r.cfg.Collectors.Cron.LogPaths,
	}
	snap := r.col.Collect(r.cfg, opt)
	r.lastSnap = &snap
	meta := agent.Meta{
		Hostname:         r.cfg.Machine.DisplayName,
		HeartbeatSeconds: int(r.cfg.Intervals.Heartbeat.Duration.Seconds()),
		UptimeSeconds:    snap.UptimeSeconds,
		SpoolQueued:      n,
		Promote:          r.cfg.Collectors.Ports.Promote,
		HeartbeatMeta:    r.heartbeatMetadata(),
		ExecPath:         clientExecPath(),
	}
	evs, next := agent.Project(snap, r.proj, meta)
	if r.cronTail != nil {
		next.CronSeen = r.cronTail.Seen
		next.CronOffsets = r.cronTail.Offsets
		next.CronPrimed = r.cronTail.Primed
	}
	r.mu.Lock()
	if r.proj != nil {
		next.AuditRunKeys = mergeAuditKeys(next.AuditRunKeys, r.proj.AuditRunKeys)
	}
	r.proj = next
	r.saveInspectState()
	r.mu.Unlock()
	for _, e := range evs {
		r.enqueue(e.Type, e.ServiceKey, e.RunKey, e.Payload)
	}
}

func (r *Runner) emitHeartbeatAlive() {
	if r.lastSnap == nil {
		r.collectAndProject()
		return
	}
	n, _ := r.sp.Count()
	r.enqueue(event.TypeHeartbeat, "", "", event.Heartbeat{
		Hostname:                 r.cfg.Machine.DisplayName,
		OS:                       "linux",
		Arch:                     "amd64",
		CollectorVersion:         "1.3.1",
		HeartbeatIntervalSeconds: int(r.cfg.Intervals.Heartbeat.Duration.Seconds()),
		UptimeSeconds:            r.col.Uptime(),
		Metadata:                 r.heartbeatMetadata(),
	})
	ttl := agent.InspectTTL
	ss := event.ServiceState{
		Name: "Host Inspect", Type: "agent", State: "running",
		Summary: "alive", Severity: "normal", TTLSeconds: &ttl,
	}
	ss.SetPath(clientExecPath())
	r.enqueue(event.TypeServiceState, agent.InspectKey, "", ss)
	r.enqueue(event.TypeStatusUpsert, selfServiceKey, "", event.StatusUpsert{
		Items: append([]event.StatusItem{
			{Key: "uptime", Label: "系统运行时间", Value: json.RawMessage(itoa(r.col.Uptime())), ValueType: "duration", Unit: "s", Severity: "normal", DisplayFormat: "duration", SortOrder: 10},
			{Key: "spool_queue", Label: "待发送队列", Value: json.RawMessage(itoa(int64(n))), ValueType: "number", Severity: sevForQueue(n), DisplayFormat: "number", SortOrder: 20},
		}, aiinspect.LoadBudget(r.sp).StatusItems()...),
	})
}

func (r *Runner) emitSelfLog(markdown, severity string) {
	r.enqueue(event.TypeLogAppend, selfServiceKey, "", event.LogPayload{Markdown: markdown, Severity: severity, Source: "board-client"})
}

func (r *Runner) maybeNotifyNewFeatures() {
	if r.sp == nil || r.cfg == nil {
		return
	}
	raw, _, _ := r.sp.GetState(config.SeenFeaturesKey)
	seen := config.ParseSeenIDs(raw)
	if len(seen) == 0 {
		seen = config.PresentIDs(r.cfg)
		_ = r.sp.SetState(config.SeenFeaturesKey, config.EncodeSeenIDs(seen))
	}
	unseen := config.UnseenIDs(seen)
	n := len(unseen)
	val, _ := json.Marshal(n)
	r.enqueue(event.TypeStatusUpsert, selfServiceKey, "", event.StatusUpsert{
		Items: []event.StatusItem{{
			Key: "config_new_features", Label: "待配置功能", Value: val,
			ValueType: "number", Severity: "info", DisplayFormat: "number", SortOrder: 30,
		}},
	})
	if n == 0 {
		return
	}
	titles := config.UnseenTitles(seen)
	if len(titles) == 0 {
		return
	}
	cfgPath := r.cfgPath
	if cfgPath == "" {
		cfgPath = "/etc/agentboard/client.yaml"
	}
	ver := strings.TrimSpace(r.Build.Version)
	if ver == "" {
		ver = "dev"
	}
	msg := "board-client " + ver + " 有新功能可配置：" + strings.Join(titles, "、") +
		"。本机运行：\n`board-client config tui --config " + cfgPath + "`\n`board-client config web --config " + cfgPath + "`"
	r.emitSelfLog(msg, "info")
}

func (r *Runner) saveInspectState() {
	if r.statePath == "" || r.proj == nil {
		return
	}
	b, _ := json.Marshal(r.proj)
	_ = os.MkdirAll(filepath.Dir(r.statePath), 0o750)
	_ = os.WriteFile(r.statePath, b, 0o600)
}

func (r *Runner) loadInspectState() {
	b, err := os.ReadFile(r.statePath)
	if err != nil {
		return
	}
	st := agent.NewState()
	if json.Unmarshal(b, st) == nil {
		r.proj = st
		if r.cronTail != nil {
			if st.CronSeen != nil {
				r.cronTail.Seen = st.CronSeen
			}
			if st.CronOffsets != nil {
				r.cronTail.Offsets = st.CronOffsets
			}
			r.cronTail.Primed = st.CronPrimed
		}
	}
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

	r.loadInspectState()
	r.CollectOnce()
	r.emitSelfLog("board-client 启动，开始采集系统快照。", "info")
	r.maybeNotifyNewFeatures()

	var wg sync.WaitGroup
	go r.compileStatusProbes(ctx)
	if err := r.startControl(ctx, &wg); err != nil {
		r.log.Warn("control socket disabled", "err", err)
	}

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	if r.cfg.LocalIngestOn() {
		srv, err := localingest.New(r.log, r.cfg.LocalIngest.Listen)
		if err != nil {
			r.log.Warn("local ingest disabled", "err", err)
		} else {
			srv.OnEvent = func(env event.Envelope) {
				r.ingestProjectCopy(env)
				if env.EventType != event.TypeLogAppend {
					return
				}
				var lp event.LogPayload
				_ = json.Unmarshal(env.Payload, &lp)
				src := lp.Source
				if src == "" {
					src = "agent_logs"
				}
				_ = r.logBuf.Append(aiinspect.Entry{
					ServiceKey: env.ServiceKey,
					Markdown:   lp.Markdown,
					Severity:   lp.Severity,
					OccurredAt: env.OccurredAt,
					Source:     src,
				})
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(ctx, r.cfg.LocalIngest.AdvertisePath); err != nil {
					r.log.Warn("local ingest stopped", "err", err)
				}
			}()
		}
	}

	wg.Add(1)
	go func() { defer wg.Done(); r.senderLoop(ctx) }()
	if r.cfg.Update.Enabled {
		wg.Add(1)
		go func() { defer wg.Done(); r.updateLoop(ctx) }()
	}

	collectEvery := r.cfg.Intervals.Collect.Duration
	if collectEvery <= 0 {
		collectEvery = time.Minute
	}
	collectT := time.NewTicker(collectEvery)
	hbT := time.NewTicker(r.cfg.Intervals.Heartbeat.Duration)
	curT := time.NewTicker(r.cfg.Intervals.CursorAgent.Duration)
	httpT := time.NewTicker(r.cfg.Intervals.HTTP.Duration)
	sideT := time.NewTicker(r.sideInterval())
	wrapT := time.NewTicker(time.Second)
	defer collectT.Stop()
	defer hbT.Stop()
	defer curT.Stop()
	defer httpT.Stop()
	defer sideT.Stop()
	defer wrapT.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-hup:
			if err := r.Reload(); err != nil {
				r.log.Warn("reload failed", "err", err)
			} else {
				r.log.Info("config reloaded")
			}
		case <-wrapT.C:
			if r.wrap != nil {
				r.wrap.TickTTL()
			}
		case <-hbT.C:
			r.emitHeartbeatAlive()
		case <-collectT.C:
			r.collectAndProject()
		case <-curT.C:
			r.emitCursorAgent()
		case <-httpT.C:
			r.emitHTTP()
		case <-sideT.C:
			r.emitProbes()
			r.emitStatusProbes()
			r.emitAISummaries()
			r.emitAIDiscover()
		}
	}
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
	ss := event.ServiceState{
		Name:     cfg.ServiceName,
		Type:     "agent",
		State:    state,
		Summary:  summary,
		Severity: sev,
	}
	ss.SetPath(lookPath("cursor-agent"))
	r.enqueue(event.TypeServiceState, cfg.ServiceKey, "", ss)
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
		if r.cfg.AI.Enabled {
			src := config.AISummarize{
				Source: "cursor_transcript", ServiceKey: cfg.ServiceKey,
				Name: cfg.ServiceName + " 日志总结", MinNewLogs: 1,
			}
			evs := aiinspect.SummarizeText(context.Background(), r.sp, r.ensureProvider(), r.cfg.AI, src, bodies, strings.Join(bodies, "\n"))
			r.enqueueOut(evs)
		} else {
			md := summarize.Logs(cfg.ServiceName+" 日志总结", bodies)
			r.enqueue(event.TypeLogPin, cfg.ServiceKey, "", event.LogPayload{
				Markdown: md,
				Severity: "info",
				Source:   "cursor-agent",
			})
		}
	}
}

func (r *Runner) emitHTTP() {
	cfg := r.cfg.Collectors.HTTP
	if !cfg.Enabled || len(cfg.Targets) == 0 {
		return
	}
	targets := make([]collector.HTTPTarget, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		targets = append(targets, collector.HTTPTarget{
			ServiceKey:     t.ServiceKey,
			Name:           t.Name,
			URL:            t.URL,
			Method:         t.Method,
			ExpectStatus:   t.ExpectStatus,
			ExpectContains: t.ExpectContains,
			Headers:        t.Headers,
			TLSInsecure:    t.TLSInsecure,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout.Duration+2*time.Second)
	defer cancel()
	results := collector.ProbeAll(ctx, cfg.Timeout.Duration, r.cfg.HTTPFollowRedirects(), targets)
	for _, res := range results {
		key := res.Target.ServiceKey
		if key == "" {
			continue
		}
		r.enqueue(event.TypeServiceState, key, "", res.ServiceState(cfg.TTLSeconds, cfg.WarnLatency.Duration))
		items := res.StatusItems(cfg.WarnLatency.Duration)
		if len(items) > 0 {
			r.enqueue(event.TypeStatusUpsert, key, "", event.StatusUpsert{Items: items})
		}
		prev, seen := r.httpPrev[key]
		r.httpPrev[key] = res.OK
		if !seen && res.OK {
			continue
		}
		if seen && prev == res.OK {
			continue
		}
		sev := "error"
		if res.OK {
			sev = "info"
		}
		r.enqueue(event.TypeLogAppend, key, "", event.LogPayload{
			Markdown: res.LogMarkdown(),
			Severity: sev,
			Source:   "http-probe",
		})
	}
}

func (r *Runner) updateLoop(ctx context.Context) {
	first := 15 * time.Second
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.maybeUpdate(ctx)
			every := r.cfg.Update.Interval.Duration
			if every <= 0 {
				every = time.Hour
			}
			t.Reset(every)
		}
	}
}

func (r *Runner) maybeUpdate(ctx context.Context) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return
	}
	timeout := r.cfg.Server.Timeout.Duration
	if timeout < 10*time.Minute {
		timeout = 10 * time.Minute
	}
	var lastErr error
	sawCurrent := false
	for _, src := range update.Sources(r.cfg.Server.URL, r.cfg.Update.URL) {
		up := update.New(src, r.Build, timeout)
		man, bin, needed, err := up.Check(ctx)
		if err != nil {
			r.log.Info("client update source skipped", "url", src, "err", err)
			lastErr = err
			continue
		}
		if !needed {
			sawCurrent = true
			r.log.Debug("board-client is current", "url", src, "commit", r.Build.Commit)
			continue
		}
		msg := "正在升级 board-client 到 " + man.Version + " (" + man.Commit + ")"
		r.log.Info("applying board-client update", "url", src, "version", man.Version, "commit", man.Commit, "asset", bin.Name)
		r.emitSelfLog(msg, "info")
		r.drainOnce(ctx)
		if err := up.Apply(ctx, bin); err != nil {
			r.log.Warn("client update failed", "url", src, "err", err)
			r.emitSelfLog("board-client 升级失败："+err.Error(), "warning")
			lastErr = err
			continue
		}
		return
	}
	if lastErr != nil && !sawCurrent {
		r.log.Warn("client update check failed", "err", lastErr)
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

func (r *Runner) ensureProvider() aiprovider.Provider {
	if r.provider != nil {
		return r.provider
	}
	if r.cfg == nil || !r.cfg.AI.Enabled {
		return nil
	}
	p, err := aiprovider.New(aiprovider.Options{
		Provider:  r.cfg.AI.Provider,
		Command:   r.cfg.AI.Command,
		Model:     r.cfg.AI.Model,
		APIKeyEnv: r.cfg.AI.APIKeyEnv,
		Workspace: r.cfg.AI.Workspace,
	})
	if err != nil {
		r.log.Warn("ai provider disabled", "err", err)
		return nil
	}
	r.provider = p
	return r.provider
}

func (r *Runner) enqueueOut(evs []aiinspect.OutEvent) {
	for _, e := range evs {
		r.enqueue(e.Type, e.ServiceKey, "", e.Payload)
	}
}

func (r *Runner) due(last time.Time, every time.Duration) bool {
	if every <= 0 {
		every = time.Minute
	}
	if last.IsZero() {
		return true
	}
	return time.Since(last) >= every
}

func (r *Runner) sideInterval() time.Duration {
	d := 15 * time.Second
	if r.cfg.AI.Enabled {
		for _, src := range r.cfg.AI.Summarize {
			if src.Interval.Duration > 0 && src.Interval.Duration < d {
				d = src.Interval.Duration
			}
		}
		if r.cfg.AI.Discover.Enabled && r.cfg.AI.Discover.Interval.Duration > 0 && r.cfg.AI.Discover.Interval.Duration < d {
			d = r.cfg.AI.Discover.Interval.Duration
		}
	}
	if r.cfg.Collectors.Probes.Enabled {
		for _, s := range r.cfg.Collectors.Probes.Scripts {
			if s.Interval.Duration > 0 && s.Interval.Duration < d {
				d = s.Interval.Duration
			}
		}
	}
	r.mu.Lock()
	for _, p := range r.statusReady {
		if p.Interval > 0 && p.Interval < d {
			d = p.Interval
		}
	}
	r.mu.Unlock()
	if d < time.Second {
		d = time.Second
	}
	return d
}

func (r *Runner) emitProbes() {
	cfg := r.cfg.Collectors.Probes
	if !cfg.Enabled {
		return
	}
	now := time.Now()
	for _, s := range cfg.Scripts {
		r.mu.Lock()
		last := r.lastProbe[s.ServiceKey]
		r.mu.Unlock()
		if !r.due(last, s.Interval.Duration) {
			continue
		}
		r.mu.Lock()
		r.lastProbe[s.ServiceKey] = now
		r.mu.Unlock()
		scriptPath := probeScriptPath(s.Command)
		out, trunc, err := probe.RunScript(context.Background(), s.Command, s.Timeout.Duration, s.MaxBytes)
		if err != nil {
			for _, e := range probe.FailedState(s.ServiceKey, s.Name, err.Error(), s.TTLSeconds) {
				r.enqueue(e.Type, e.ServiceKey, "", withServicePath(e.Payload, scriptPath))
			}
			r.enqueue(event.TypeCollectorNotice, "", "", event.CollectorNotice{
				Severity: "warning", Code: "probe_failed",
				Markdown: "probe " + s.ServiceKey + ": " + err.Error(),
			})
			continue
		}
		if strings.EqualFold(s.Format, "text") {
			text := string(out)
			if trunc {
				text += "\n…(truncated)"
			}
			ttl := s.TTLSeconds
			ss := event.ServiceState{
				Name: s.Name, Type: "virtual", State: "running",
				Summary: "probe ok", Severity: "normal", TTLSeconds: &ttl,
			}
			ss.SetPath(scriptPath)
			r.enqueue(event.TypeServiceState, s.ServiceKey, "", ss)
			appendLog := s.AppendLog == nil || *s.AppendLog
			if appendLog {
				r.enqueue(event.TypeLogAppend, s.ServiceKey, "", event.LogPayload{
					Markdown: text, Severity: "info", Source: "probe",
				})
			}
			_ = r.logBuf.Append(aiinspect.Entry{
				ServiceKey: s.ServiceKey, Markdown: text, Severity: "info",
				Source: "probe:" + s.ServiceKey,
			})
			continue
		}
		parsed, err := probe.ParseJSON(out)
		if err != nil {
			for _, e := range probe.FailedState(s.ServiceKey, s.Name, err.Error(), s.TTLSeconds) {
				r.enqueue(e.Type, e.ServiceKey, "", withServicePath(e.Payload, scriptPath))
			}
			continue
		}
		if trunc {
			parsed.Summary += " (truncated)"
		}
		r.mu.Lock()
		prev := r.probePin[s.ServiceKey]
		r.mu.Unlock()
		evs, newHash := probe.MapJSON(s.ServiceKey, s.Name, s.TTLSeconds, parsed, prev)
		r.mu.Lock()
		r.probePin[s.ServiceKey] = newHash
		r.mu.Unlock()
		for _, e := range evs {
			r.enqueue(e.Type, e.ServiceKey, "", withServicePath(e.Payload, scriptPath))
		}
	}
}

func (r *Runner) emitAISummaries() {
	if !r.cfg.AI.Enabled {
		return
	}
	p := r.ensureProvider()
	now := time.Now()
	for _, src := range r.cfg.AI.Summarize {
		r.mu.Lock()
		last := r.lastSum[src.ServiceKey]
		r.mu.Unlock()
		if !r.due(last, src.Interval.Duration) {
			continue
		}
		r.mu.Lock()
		r.lastSum[src.ServiceKey] = now
		r.mu.Unlock()
		var evs []aiinspect.OutEvent
		if src.Source == "cursor_transcript" {
			files := collector.ScanTranscripts(r.cfg.Collectors.CursorAgent.Paths)
			bodies, joined := aiinspect.TranscriptBodies(files)
			evs = aiinspect.SummarizeText(context.Background(), r.sp, p, r.cfg.AI, src, bodies, joined)
		} else {
			evs = aiinspect.SummarizeOne(context.Background(), r.sp, r.logBuf, p, r.cfg.AI, src)
		}
		r.enqueueOut(evs)
	}
}

func (r *Runner) emitAIDiscover() {
	if !r.cfg.AI.Enabled || !r.cfg.AI.Discover.Enabled {
		return
	}
	every := r.cfg.AI.Discover.Interval.Duration
	if every <= 0 {
		every = r.cfg.Intervals.AIDiscover.Duration
	}
	if !r.due(r.lastDisc, every) {
		return
	}
	r.lastDisc = time.Now()
	evs := aiinspect.Discover(context.Background(), r.sp, r.ensureProvider(), r.cfg.AI, r.runCmd)
	r.enqueueOut(evs)
}

func clientExecPath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(p); err == nil && real != "" {
		return real
	}
	return p
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

func probeScriptPath(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	return cmd[0]
}

func withServicePath(payload any, path string) any {
	ss, ok := payload.(event.ServiceState)
	if !ok {
		return payload
	}
	ss.SetPath(path)
	return ss
}
