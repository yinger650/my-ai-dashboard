package runner

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agentboard/internal/client/config"
	"agentboard/internal/client/probe"
	"agentboard/internal/client/statusprobe"
	"agentboard/internal/event"
)

const statusFailLimit = 3

func (r *Runner) compileStatusProbes(ctx context.Context) {
	if r.cfg == nil {
		return
	}
	probes := append([]config.StatusProbe(nil), r.cfg.Machine.StatusProbes...)
	if len(probes) == 0 {
		r.setStatusReady(nil, true)
		return
	}
	var hand, gen []config.StatusProbe
	for _, p := range probes {
		if len(p.Command) > 0 {
			hand = append(hand, p)
		} else {
			gen = append(gen, p)
		}
	}
	comp := &statusprobe.Compiler{
		Dir:       r.cfg.ProbeDir(),
		Provider:  r.ensureProvider(),
		AIEnabled: r.cfg.AI.Enabled,
		Notice: func(code, md string) {
			r.enqueue(event.TypeCollectorNotice, "", "", event.CollectorNotice{
				Severity: "warning", Code: code, Markdown: md,
			})
		},
	}
	ready := comp.Prepare(ctx, hand)
	if len(ready) > 0 {
		r.setStatusReady(mergeReady(ready, r.snapshotReady()), false)
		r.emitStatusProbes()
	}
	if len(gen) > 0 {
		ready = append(ready, comp.Prepare(ctx, gen)...)
	}
	r.setStatusReady(ready, true)
}

func mergeReady(extra []statusprobe.Ready, prev []statusprobe.Ready) []statusprobe.Ready {
	seen := map[string]int{}
	var out []statusprobe.Ready
	for _, p := range prev {
		seen[p.Key] = len(out)
		out = append(out, p)
	}
	for _, p := range extra {
		if i, ok := seen[p.Key]; ok {
			out[i] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

func (r *Runner) snapshotReady() []statusprobe.Ready {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]statusprobe.Ready, 0, len(r.statusReady))
	for _, p := range r.statusReady {
		out = append(out, p.Ready)
	}
	return out
}

func (r *Runner) setStatusReady(ready []statusprobe.Ready, dropMissing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.statusReady == nil {
		r.statusReady = map[string]*liveStatus{}
	}
	if r.hbMeta == nil {
		r.hbMeta = map[string]any{}
	}
	live := map[string]struct{}{}
	for _, p := range ready {
		live[p.Key] = struct{}{}
		if cur, ok := r.statusReady[p.Key]; ok {
			cur.Ready = p
			continue
		}
		r.statusReady[p.Key] = &liveStatus{Ready: p}
	}
	if !dropMissing {
		return
	}
	for key, st := range r.statusReady {
		if _, ok := live[key]; ok {
			continue
		}
		for _, mk := range r.probeMetaKeys[key] {
			r.hbMeta[mk] = nil
		}
		delete(r.probeMetaKeys, key)
		delete(r.statusReady, key)
		_ = st
	}
}

func (r *Runner) heartbeatMetadata() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.hbMeta) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range r.hbMeta {
		out[k] = v
		if v == nil {
			delete(r.hbMeta, k)
		}
	}
	return out
}

func (r *Runner) emitStatusProbes() {
	r.mu.Lock()
	items := make([]*liveStatus, 0, len(r.statusReady))
	for _, p := range r.statusReady {
		items = append(items, p)
	}
	r.mu.Unlock()
	now := time.Now()
	for _, st := range items {
		r.mu.Lock()
		last := r.lastProbe["sp:"+st.Key]
		r.mu.Unlock()
		if !r.due(last, st.Interval) {
			continue
		}
		r.mu.Lock()
		r.lastProbe["sp:"+st.Key] = now
		r.mu.Unlock()
		out, _, err := probe.RunScript(context.Background(), st.Command, st.Timeout, 0)
		if err != nil {
			r.statusProbeFail(st, err.Error())
			continue
		}
		parsed, err := probe.ParseJSON(out)
		if err != nil {
			r.statusProbeFail(st, err.Error())
			continue
		}
		r.statusProbeOK(st, parsed)
	}
}

func (r *Runner) statusProbeFail(st *liveStatus, msg string) {
	r.enqueue(event.TypeCollectorNotice, "", "", event.CollectorNotice{
		Severity: "warning", Code: "status_probe_failed",
		Markdown: "status_probe " + st.Key + ": " + msg,
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	st.fails++
	if st.fails < statusFailLimit {
		return
	}
	for _, mk := range r.probeMetaKeys[st.Key] {
		r.hbMeta[mk] = nil
	}
}

func (r *Runner) statusProbeOK(st *liveStatus, parsed probe.Result) {
	nums := statusprobe.NumericMeta(st.Key, parsed.Statuses)
	non := statusprobe.NonNumericStatuses(parsed.Statuses)
	r.mu.Lock()
	st.fails = 0
	var keys []string
	for k, v := range nums {
		r.hbMeta[k] = v
		keys = append(keys, k)
	}
	prev := r.probeMetaKeys[st.Key]
	for _, old := range prev {
		if _, ok := nums[old]; !ok {
			r.hbMeta[old] = nil
		}
	}
	r.probeMetaKeys[st.Key] = keys
	r.mu.Unlock()
	if len(non) == 0 {
		return
	}
	items := make([]event.StatusItem, 0, len(non))
	for i, s := range non {
		val, _ := json.Marshal(s.Value)
		label := s.Label
		if label == "" {
			label = s.Key
		}
		items = append(items, event.StatusItem{
			Key: s.Key, Label: label, Value: val, ValueType: "string",
			Unit: s.Unit, Severity: s.Severity, DisplayFormat: "text", SortOrder: 100 + (i+1)*10,
		})
	}
	r.enqueue(event.TypeStatusUpsert, selfServiceKey, "", event.StatusUpsert{Items: items})
}

func clipRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
