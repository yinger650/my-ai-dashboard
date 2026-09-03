package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"agentboard/internal/event"
)

const httpProbeUserAgent = "AgentBoard-Client/1.2 (+https://board.yinger650.com)"
const httpMaxBody = 64 * 1024

// HTTPTarget is one website (or health URL) to probe.
type HTTPTarget struct {
	ServiceKey     string
	Name           string
	URL            string
	Method         string
	ExpectStatus   []int
	ExpectContains string
	Headers        map[string]string
	TLSInsecure    bool
}

// HTTPResult is the outcome of one probe.
type HTTPResult struct {
	Target      HTTPTarget
	OK          bool
	StatusCode  int
	Latency     time.Duration
	Err         string
	SSLDaysLeft *int
	Summary     string
}

// ProbeAll runs every target concurrently and returns results in input order.
func ProbeAll(ctx context.Context, timeout time.Duration, followRedirects bool, targets []HTTPTarget) []HTTPResult {
	out := make([]HTTPResult, len(targets))
	if len(targets) == 0 {
		return out
	}
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t HTTPTarget) {
			defer wg.Done()
			out[i] = ProbeHTTP(ctx, timeout, followRedirects, t)
		}(i, t)
	}
	wg.Wait()
	return out
}

// ProbeHTTP issues one HTTP(S) request and classifies success vs failure.
func ProbeHTTP(ctx context.Context, timeout time.Duration, followRedirects bool, t HTTPTarget) HTTPResult {
	res := HTTPResult{Target: t}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	method := strings.ToUpper(strings.TrimSpace(t.Method))
	if method == "" {
		method = http.MethodGet
	}
	expect := t.ExpectStatus
	if len(expect) == 0 {
		expect = []int{http.StatusOK}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, t.URL, nil)
	if err != nil {
		res.Err = err.Error()
		res.Summary = "请求无效：" + truncateErr(err.Error())
		return res
	}
	req.Header.Set("User-Agent", httpProbeUserAgent)
	for k, v := range t.Headers {
		if k != "" {
			req.Header.Set(k, v)
		}
	}

	client := newProbeClient(timeout, followRedirects, t.TLSInsecure)
	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err.Error()
		if reqCtx.Err() == context.DeadlineExceeded || strings.Contains(err.Error(), "context deadline exceeded") {
			res.Summary = fmt.Sprintf("超时（%s）", timeout)
		} else {
			res.Summary = "连接失败：" + truncateErr(err.Error())
		}
		return res
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		days := int(time.Until(resp.TLS.PeerCertificates[0].NotAfter).Hours() / 24)
		res.SSLDaysLeft = &days
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, httpMaxBody+1))
	wantStatus := containsInt(expect, resp.StatusCode)
	wantBody := t.ExpectContains == "" || strings.Contains(string(body), t.ExpectContains)
	res.OK = wantStatus && wantBody
	lat := formatLatency(res.Latency)
	switch {
	case !wantStatus:
		res.Summary = fmt.Sprintf("HTTP %d（期望 %s）· %s", resp.StatusCode, joinInts(expect), lat)
	case !wantBody:
		res.Summary = fmt.Sprintf("HTTP %d · 响应未包含 %q · %s", resp.StatusCode, t.ExpectContains, lat)
	default:
		res.Summary = fmt.Sprintf("HTTP %d · %s", resp.StatusCode, lat)
	}
	return res
}

func newProbeClient(timeout time.Duration, followRedirects, tlsInsecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsInsecure {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		} else {
			transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
	c := &http.Client{Timeout: timeout, Transport: transport}
	if !followRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return c
}

// Projection maps a probe onto service.state fields.
func (r HTTPResult) Projection(warnLatency time.Duration) (state, summary, severity string) {
	summary = r.Summary
	if !r.OK {
		return "failed", summary, "error"
	}
	severity = "normal"
	if warnLatency > 0 && r.Latency >= warnLatency {
		severity = "warning"
		summary += "（偏慢）"
	}
	if r.SSLDaysLeft != nil {
		days := *r.SSLDaysLeft
		if days < 14 {
			summary += fmt.Sprintf(" · 证书剩余 %d 天", days)
			if severity == "normal" {
				severity = "warning"
			}
		}
	}
	return "running", summary, severity
}

// ServiceState builds a service.state payload for this probe.
func (r HTTPResult) ServiceState(ttlSeconds int, warnLatency time.Duration) event.ServiceState {
	state, summary, severity := r.Projection(warnLatency)
	name := r.Target.Name
	if name == "" {
		name = r.Target.ServiceKey
	}
	ss := event.ServiceState{
		Name:     name,
		Type:     "virtual",
		State:    state,
		Summary:  summary,
		Severity: severity,
		Metadata: map[string]any{"url": r.Target.URL},
	}
	ss.SetPath(r.Target.URL)
	if ttlSeconds > 0 {
		ss.TTLSeconds = &ttlSeconds
	}
	return ss
}

// StatusItems builds status.upsert items for this probe.
func (r HTTPResult) StatusItems(warnLatency time.Duration) []event.StatusItem {
	probeVal, probeSev := "down", "error"
	if r.OK {
		probeVal, probeSev = "up", "normal"
	}
	statusSev := "error"
	if r.OK {
		statusSev = "normal"
	} else if r.StatusCode > 0 {
		statusSev = "error"
	}
	latSev := "error"
	if r.OK {
		latSev = "normal"
		if warnLatency > 0 && r.Latency >= warnLatency {
			latSev = "warning"
		}
	}
	items := []event.StatusItem{
		{Key: "probe", Label: "探测结果", Value: jsonVal(probeVal), ValueType: "string", Severity: probeSev, DisplayFormat: "text", SortOrder: 10},
		{Key: "http_status", Label: "HTTP 状态码", Value: jsonVal(r.StatusCode), ValueType: "number", Severity: statusSev, DisplayFormat: "number", SortOrder: 20},
		{Key: "latency_ms", Label: "响应时间", Value: jsonVal(r.Latency.Milliseconds()), ValueType: "number", Unit: "ms", Severity: latSev, DisplayFormat: "number", SortOrder: 30},
	}
	if r.SSLDaysLeft != nil {
		days := *r.SSLDaysLeft
		sslSev := "normal"
		if days < 7 {
			sslSev = "error"
		} else if days < 14 {
			sslSev = "warning"
		}
		items = append(items, event.StatusItem{
			Key: "ssl_days", Label: "证书剩余天数", Value: jsonVal(days), ValueType: "number", Unit: "d",
			Severity: sslSev, DisplayFormat: "number", SortOrder: 40,
		})
	}
	return items
}

// LogMarkdown describes a probe transition for log.append.
func (r HTTPResult) LogMarkdown() string {
	name := r.Target.Name
	if name == "" {
		name = r.Target.ServiceKey
	}
	if r.OK {
		return fmt.Sprintf("**%s** 恢复正常：[%s](%s) · %s", name, r.Target.URL, r.Target.URL, r.Summary)
	}
	extra := r.Summary
	if r.Err != "" {
		extra = extra + "\n\n```\n" + truncateErr(r.Err) + "\n```"
	}
	return fmt.Sprintf("**%s** 探测失败：[%s](%s)\n\n%s", name, r.Target.URL, r.Target.URL, extra)
}

func jsonVal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

func joinInts(list []int) string {
	parts := make([]string, len(list))
	for i, n := range list {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, "/")
}

func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}
