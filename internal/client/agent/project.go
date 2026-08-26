// Package agent is the Part 2 host-inspect projector: it turns a HostSnapshot
// into Board events. The agent service itself only reports liveness.
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/hostsnap"
	"agentboard/internal/event"
)

const (
	InspectKey = "host-inspect"
	ListenKey  = "host-listen"
	DockerKey  = "docker"
	CronKey    = "cron"
	NginxKey   = "nginx"
	SelfKey    = "board-client"
	InspectTTL = 180
)

// Event is one projected ingest event (payload not yet enveloped).
type Event struct {
	Type       string
	ServiceKey string
	RunKey     string
	Payload    any
}

// Meta is machine-level context needed to project.
type Meta struct {
	Hostname         string
	HeartbeatSeconds int
	UptimeSeconds    int64
	SpoolQueued      int
	Arch             string
	Promote          []config.PromoteRule
}

// State is remembered across collect rounds so pins and logs only fire on change.
type State struct {
	Pins             map[string]string `json:"pins"`
	DockerContainers map[string]string `json:"docker_containers"`
	NginxPID         int               `json:"nginx_pid"`
	NginxHadProxies  bool              `json:"nginx_had_proxies"`
	UnitActive       map[string]string `json:"unit_active"`
	CronSeen         map[string]bool   `json:"cron_seen"`
	CronOffsets      map[string]int64  `json:"cron_offsets"`
	CronPrimed       bool              `json:"cron_primed"`
}

// NewState returns an empty projection state.
func NewState() *State {
	return &State{
		Pins:             map[string]string{},
		DockerContainers: map[string]string{},
		UnitActive:       map[string]string{},
		CronSeen:         map[string]bool{},
		CronOffsets:      map[string]int64{},
	}
}

// Project maps one snapshot into Board events and an updated state.
func Project(snap hostsnap.Snapshot, prev *State, meta Meta) ([]Event, *State) {
	if prev == nil {
		prev = NewState()
	}
	next := &State{
		Pins:             copyMap(prev.Pins),
		DockerContainers: copyMap(prev.DockerContainers),
		NginxPID:         prev.NginxPID,
		NginxHadProxies:  prev.NginxHadProxies,
		UnitActive:       copyMap(prev.UnitActive),
		CronSeen:         copyBoolMap(prev.CronSeen),
		CronOffsets:      copyInt64Map(prev.CronOffsets),
		CronPrimed:       prev.CronPrimed,
	}
	var evs []Event
	evs = append(evs, machineEvents(snap, meta)...)
	evs = append(evs, inspectAlive()...)
	evs = append(evs, selfEvents(meta)...)
	evs = append(evs, projectPorts(snap, next, meta)...)
	evs = append(evs, projectUnits(snap, next)...)
	evs = append(evs, projectDocker(snap, prev, next)...)
	evs = append(evs, projectCron(snap, next)...)
	evs = append(evs, projectNginx(snap, prev, next)...)
	return evs, next
}

func machineEvents(snap hostsnap.Snapshot, meta Meta) []Event {
	arch := meta.Arch
	if arch == "" {
		arch = "amd64"
	}
	return []Event{
		{Type: event.TypeHeartbeat, Payload: event.Heartbeat{
			Hostname:                 meta.Hostname,
			OS:                       "linux",
			Arch:                     arch,
			CollectorVersion:         "1.3.1",
			HeartbeatIntervalSeconds: meta.HeartbeatSeconds,
			UptimeSeconds:            meta.UptimeSeconds,
		}},
		{Type: event.TypeMetricSample, Payload: snap.Metric},
	}
}

func inspectAlive() []Event {
	ttl := InspectTTL
	return []Event{
		{Type: event.TypeServiceState, ServiceKey: InspectKey, Payload: event.ServiceState{
			Name: "Host Inspect", Type: "agent", State: "running",
			Summary: "alive", Severity: "normal", TTLSeconds: &ttl,
		}},
		{Type: event.TypeStatusUpsert, ServiceKey: InspectKey, Payload: event.StatusUpsert{
			Items: []event.StatusItem{
				{Key: "alive", Label: "存活", Value: rawBool(true), ValueType: "boolean", Severity: "normal", DisplayFormat: "text", SortOrder: 10},
			},
		}},
	}
}

func selfEvents(meta Meta) []Event {
	return []Event{
		{Type: event.TypeServiceState, ServiceKey: SelfKey, Payload: event.ServiceState{
			Name: "Board Client", Type: "daemon", State: "running",
			Summary: "collecting snapshot", Severity: "normal",
		}},
		{Type: event.TypeStatusUpsert, ServiceKey: SelfKey, Payload: event.StatusUpsert{
			Items: []event.StatusItem{
				{Key: "uptime", Label: "系统运行时间", Value: rawInt(meta.UptimeSeconds), ValueType: "duration", Unit: "s", Severity: "normal", DisplayFormat: "duration", SortOrder: 10},
				{Key: "spool_queue", Label: "待发送队列", Value: rawInt(int64(meta.SpoolQueued)), ValueType: "number", Severity: queueSev(meta.SpoolQueued), DisplayFormat: "number", SortOrder: 20},
			},
		}},
	}
}

func projectPorts(snap hostsnap.Snapshot, st *State, meta Meta) []Event {
	if snap.Ports == nil {
		return nil
	}
	var evs []Event
	evs = append(evs, Event{Type: event.TypePortSnapshot, Payload: map[string]any{"ports": snap.Ports}})

	md := renderPortTable(snap.Ports)
	evs = append(evs, pinIfChanged(st, ListenKey, "Host Listen", "virtual", md, "info")...)

	type agg struct {
		key, name string
		items     []event.StatusItem
	}
	groups := map[string]*agg{}
	order := []string{}
	for _, p := range snap.Ports {
		rule := matchPromote(p.Process, meta.Promote)
		if rule == nil {
			continue
		}
		g, ok := groups[rule.ServiceKey]
		if !ok {
			g = &agg{key: rule.ServiceKey, name: rule.Name}
			groups[rule.ServiceKey] = g
			order = append(order, rule.ServiceKey)
		}
		k := "listen_" + strconv.Itoa(p.Port)
		val := p.Address + ":" + strconv.Itoa(p.Port) + "/" + p.Protocol
		g.items = append(g.items, event.StatusItem{
			Key: k, Label: "监听 " + strconv.Itoa(p.Port), Value: rawString(val),
			ValueType: "string", Severity: "normal", DisplayFormat: "text", SortOrder: 10 + p.Port,
		})
	}
	for _, key := range order {
		g := groups[key]
		evs = append(evs, Event{Type: event.TypeServiceState, ServiceKey: g.key, Payload: event.ServiceState{
			Name: g.name, Type: "daemon", State: "running", Summary: "listening", Severity: "normal",
		}})
		if len(g.items) > 0 {
			evs = append(evs, Event{Type: event.TypeStatusUpsert, ServiceKey: g.key, Payload: event.StatusUpsert{Items: g.items}})
		}
	}
	return evs
}

func matchPromote(process string, rules []config.PromoteRule) *config.PromoteRule {
	if process == "" {
		return nil
	}
	base := process
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for i := range rules {
		if rules[i].Process == base || rules[i].Process == process {
			r := rules[i]
			if r.ServiceKey == "" {
				r.ServiceKey = strings.ToLower(base)
			}
			if r.Name == "" {
				r.Name = base
			}
			return &r
		}
	}
	return nil
}

func renderPortTable(ports []hostsnap.Port) string {
	type row struct{ svc, ports, bind, proc string }
	by := map[string]*row{}
	order := []string{}
	for _, p := range ports {
		proc := p.Process
		if proc == "" {
			proc = "-"
		}
		key := proc
		r, ok := by[key]
		if !ok {
			r = &row{svc: proc, bind: p.Address, proc: proc}
			by[key] = r
			order = append(order, key)
		}
		ps := strconv.Itoa(p.Port)
		if r.ports == "" {
			r.ports = ps
		} else if !strings.Contains(","+r.ports+",", ","+ps+",") {
			r.ports += ", " + ps
		}
		if r.bind != p.Address && p.Address != "" {
			r.bind = "*"
		}
	}
	var b strings.Builder
	b.WriteString("| 服务 | 端口 | 绑定 | 进程 |\n| --- | --- | --- | --- |\n")
	for _, k := range order {
		r := by[k]
		b.WriteString("| " + r.svc + " | " + r.ports + " | " + r.bind + " | " + r.proc + " |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func projectUnits(snap hostsnap.Snapshot, st *State) []Event {
	var evs []Event
	for _, u := range snap.Units {
		key := normalizeUnitKey(u.Unit)
		if key == "" || dedicatedUnitKey(key) {
			continue
		}
		state, summary, sev := event.UnitProjection(u.Active, u.Sub, u.Description)
		name := u.Description
		if name == "" {
			name = key
		}
		evs = append(evs, Event{Type: event.TypeServiceState, ServiceKey: key, Payload: event.ServiceState{
			Name: name, Type: "daemon", State: state, Summary: summary, Severity: sev,
		}})
		prev := st.UnitActive[u.Unit]
		st.UnitActive[u.Unit] = u.Active + "/" + u.Sub
		if prev != "" && prev != st.UnitActive[u.Unit] && (u.Active == "failed" || u.Active == "inactive") {
			evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: key, Payload: event.LogPayload{
				Markdown: name + " 状态变为 **" + u.Active + "/" + u.Sub + "**。",
				Severity: sev,
				Source:   "systemd",
			}})
		}
	}
	return evs
}

func normalizeUnitKey(unit string) string {
	key := strings.TrimSuffix(unit, ".service")
	switch key {
	case "crond", "cronie":
		return CronKey
	case "docker":
		return DockerKey
	case "board-client":
		return SelfKey
	}
	if !event.ValidServiceKey(key) {
		return ""
	}
	return key
}

func dedicatedUnitKey(key string) bool {
	switch key {
	case SelfKey, CronKey, NginxKey, DockerKey, InspectKey, ListenKey:
		return true
	}
	return false
}

func projectDocker(snap hostsnap.Snapshot, prev, next *State) []Event {
	if snap.Docker == nil {
		return nil
	}
	if !snap.Docker.Available {
		return []Event{{Type: event.TypeServiceState, ServiceKey: DockerKey, Payload: event.ServiceState{
			Name: "Docker", Type: "daemon", State: "unknown",
			Summary: "未安装或未运行", Severity: "unknown",
		}}}
	}
	running, stopped := 0, 0
	cur := map[string]string{}
	var runRows []hostsnap.Container
	for _, c := range snap.Docker.Containers {
		cur[c.ID] = c.State
		if c.State == "running" {
			running++
			runRows = append(runRows, c)
		} else {
			stopped++
		}
	}
	next.DockerContainers = cur
	var evs []Event
	evs = append(evs, Event{Type: event.TypeServiceState, ServiceKey: DockerKey, Payload: event.ServiceState{
		Name: "Docker", Type: "daemon", State: "running",
		Summary:  "运行 " + strconv.Itoa(running) + " · 停止 " + strconv.Itoa(stopped) + " · 镜像 " + strconv.Itoa(snap.Docker.ImageCount),
		Severity: "normal",
	}})
	evs = append(evs, Event{Type: event.TypeStatusUpsert, ServiceKey: DockerKey, Payload: event.StatusUpsert{
		Items: []event.StatusItem{
			{Key: "running", Label: "运行中", Value: rawInt(int64(running)), ValueType: "number", Severity: "normal", DisplayFormat: "number", SortOrder: 10},
			{Key: "stopped", Label: "已停止", Value: rawInt(int64(stopped)), ValueType: "number", Severity: "info", DisplayFormat: "number", SortOrder: 20},
			{Key: "images", Label: "镜像", Value: rawInt(int64(snap.Docker.ImageCount)), ValueType: "number", Severity: "info", DisplayFormat: "number", SortOrder: 30},
		},
	}})
	md := renderDockerTable(running, stopped, snap.Docker.ImageCount, runRows)
	evs = append(evs, pinIfChanged(next, DockerKey, "Docker", "daemon", md, "info")...)

	// diffs
	if len(prev.DockerContainers) == 0 {
		return evs
	}
	for id, st := range cur {
		old, ok := prev.DockerContainers[id]
		if !ok {
			name := containerName(snap.Docker.Containers, id)
			evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: DockerKey, Payload: event.LogPayload{
				Markdown: "容器 **" + name + "** 已创建（" + st + "）。",
				Severity: "info", Source: "docker",
			}})
			continue
		}
		if old != st {
			name := containerName(snap.Docker.Containers, id)
			evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: DockerKey, Payload: event.LogPayload{
				Markdown: "容器 **" + name + "** " + old + " → " + st + "。",
				Severity: dockerSev(st), Source: "docker",
			}})
		}
	}
	for id := range prev.DockerContainers {
		if _, ok := cur[id]; !ok {
			evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: DockerKey, Payload: event.LogPayload{
				Markdown: "容器 `" + shortID(id) + "` 已删除。",
				Severity: "info", Source: "docker",
			}})
		}
	}
	return evs
}

func renderDockerTable(running, stopped, images int, rows []hostsnap.Container) string {
	var b strings.Builder
	b.WriteString("运行 " + strconv.Itoa(running) + " · 停止 " + strconv.Itoa(stopped) + " · 镜像 " + strconv.Itoa(images) + "\n\n")
	if len(rows) == 0 {
		b.WriteString("当前没有运行中的容器。")
		return b.String()
	}
	b.WriteString("| 容器 | 镜像 | 端口 |\n| --- | --- | --- |\n")
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	for _, c := range rows {
		ports := c.Ports
		if ports == "" {
			ports = "-"
		}
		if len(ports) > 48 {
			ports = ports[:45] + "..."
		}
		b.WriteString("| " + c.Name + " | " + c.Image + " | " + ports + " |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func projectCron(snap hostsnap.Snapshot, st *State) []Event {
	if snap.Cron == nil {
		return nil
	}
	jobs := make([]hostsnap.CronJob, 0, len(snap.Cron.Jobs))
	for _, j := range snap.Cron.Jobs {
		if collector.IsCronNoise(j.Command) {
			continue
		}
		jobs = append(jobs, j)
	}
	var evs []Event
	n := len(jobs)
	evs = append(evs, Event{Type: event.TypeServiceState, ServiceKey: CronKey, Payload: event.ServiceState{
		Name: "Cron", Type: "scheduled", State: "running",
		Summary: strconv.Itoa(n) + " 条计划", Severity: "normal",
	}})
	evs = append(evs, Event{Type: event.TypeStatusUpsert, ServiceKey: CronKey, Payload: event.StatusUpsert{
		Items: []event.StatusItem{
			{Key: "jobs", Label: "计划数", Value: rawInt(int64(n)), ValueType: "number", Severity: "normal", DisplayFormat: "number", SortOrder: 10},
		},
	}})
	evs = append(evs, pinIfChanged(st, CronKey, "Cron", "scheduled", renderCronTable(jobs), "info")...)

	for _, ex := range snap.Cron.Executions {
		if collector.IsCronNoise(ex.Command) || st.CronSeen[ex.Key] {
			continue
		}
		st.CronSeen[ex.Key] = true
		status := "succeeded"
		sev := "info"
		if ex.Succeeded != nil && !*ex.Succeeded {
			status = "failed"
			sev = "error"
		}
		summary := ex.User + " " + truncate(ex.Command, 80)
		evs = append(evs, Event{
			Type: event.TypeRunTransition, ServiceKey: CronKey, RunKey: "cron-" + ex.Key,
			Payload: event.RunTransition{
				ServiceName: "Cron", ServiceType: "scheduled", Status: status,
				Summary: summary, StartedAt: ex.Occurred, FinishedAt: ex.Occurred,
			},
		})
		evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: CronKey, RunKey: "cron-" + ex.Key, Payload: event.LogPayload{
			Markdown: cronExecMarkdown(ex),
			Severity: sev,
			Source:   "cron",
		}})
	}
	return evs
}

func renderCronTable(jobs []hostsnap.CronJob) string {
	if len(jobs) == 0 {
		return "当前没有启用的 cron 计划。"
	}
	var b strings.Builder
	b.WriteString("| 时刻 | 用户 | 任务 |\n| --- | --- | --- |\n")
	for _, j := range jobs {
		user := j.User
		if user == "" {
			user = "-"
		}
		b.WriteString("| " + escapeCell(j.Schedule) + " | " + escapeCell(user) + " | " + escapeCell(truncate(j.Command, 80)) + " |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func cronExecMarkdown(ex hostsnap.CronExec) string {
	user := ex.User
	if user == "" {
		user = "-"
	}
	when := ex.Occurred
	if when == "" {
		when = "刚刚"
	}
	result := "完成"
	if ex.Succeeded != nil && !*ex.Succeeded {
		result = "失败"
	}
	return when + " **" + user + "** `" + truncate(ex.Command, 100) + "` " + result + "。"
}

func projectNginx(snap hostsnap.Snapshot, prev, next *State) []Event {
	if snap.Nginx == nil {
		return nil
	}
	effective := collector.EffectiveProxies(snap.Nginx.Proxies, snap.Ports)
	next.NginxHadProxies = len(effective) > 0
	if snap.Nginx.PID > 0 {
		next.NginxPID = snap.Nginx.PID
	}
	var evs []Event
	state, sev, summary := "running", "normal", strconv.Itoa(len(effective))+" 条生效反代"
	if !snap.Nginx.Available && len(effective) == 0 {
		state, sev, summary = "unknown", "unknown", "未发现 nginx 配置"
	}
	evs = append(evs, Event{Type: event.TypeServiceState, ServiceKey: NginxKey, Payload: event.ServiceState{
		Name: "Nginx", Type: "daemon", State: state, Summary: summary, Severity: sev,
	}})
	evs = append(evs, Event{Type: event.TypeStatusUpsert, ServiceKey: NginxKey, Payload: event.StatusUpsert{
		Items: []event.StatusItem{
			{Key: "proxies", Label: "生效反代", Value: rawInt(int64(len(effective))), ValueType: "number", Severity: "normal", DisplayFormat: "number", SortOrder: 10},
		},
	}})
	md := renderNginxTable(effective)
	evs = append(evs, pinIfChanged(next, NginxKey, "Nginx", "daemon", md, "info")...)

	if prev.NginxPID > 0 && snap.Nginx.PID > 0 && prev.NginxPID != snap.Nginx.PID {
		evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: NginxKey, Payload: event.LogPayload{
			Markdown: "Nginx 已重启（PID " + strconv.Itoa(prev.NginxPID) + " → " + strconv.Itoa(snap.Nginx.PID) + "）。",
			Severity: "info", Source: "nginx",
		}})
	}
	if prev.NginxHadProxies && len(effective) == 0 {
		evs = append(evs, Event{Type: event.TypeLogAppend, ServiceKey: NginxKey, Payload: event.LogPayload{
			Markdown: "全部反代已隐藏：当前没有监听中的生效代理。",
			Severity: "warning", Source: "nginx",
		}})
	}
	return evs
}

func renderNginxTable(proxies []hostsnap.Proxy) string {
	if len(proxies) == 0 {
		return "当前没有生效的反代。"
	}
	var b strings.Builder
	b.WriteString("| server | listen | location | upstream |\n| --- | --- | --- | --- |\n")
	for _, p := range proxies {
		loc := p.Location
		if loc == "" {
			loc = "/"
		}
		name := p.ServerName
		if name == "" {
			name = "_"
		}
		b.WriteString("| " + escapeCell(name) + " | " + escapeCell(p.Listen) + " | " + escapeCell(loc) + " | " + escapeCell(p.Upstream) + " |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func pinIfChanged(st *State, key, name, typ, markdown, sev string) []Event {
	sum := sha256.Sum256([]byte(markdown))
	h := hex.EncodeToString(sum[:])
	if st.Pins[key] == h {
		return nil
	}
	st.Pins[key] = h
	out := []Event{{Type: event.TypeLogPin, ServiceKey: key, Payload: event.LogPayload{
		Markdown: markdown, Severity: sev, Source: "host-inspect",
	}}}
	if key == ListenKey {
		out = append([]Event{{Type: event.TypeServiceState, ServiceKey: key, Payload: event.ServiceState{
			Name: name, Type: typ, State: "running", Summary: "listening ports", Severity: "normal",
		}}}, out...)
	}
	return out
}

func rawInt(v int64) json.RawMessage { return json.RawMessage(strconv.FormatInt(v, 10)) }
func rawBool(v bool) json.RawMessage {
	if v {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}
func rawString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}
func queueSev(n int) string {
	if n > 10000 {
		return "warning"
	}
	return "normal"
}
func dockerSev(state string) string {
	if state == "exited" || state == "dead" {
		return "warning"
	}
	return "info"
}
func containerName(cs []hostsnap.Container, id string) string {
	for _, c := range cs {
		if c.ID == id {
			if c.Name != "" {
				return c.Name
			}
			return shortID(id)
		}
	}
	return shortID(id)
}
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
func copyMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func copyBoolMap(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func copyInt64Map(in map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
