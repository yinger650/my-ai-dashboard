package cfgui

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
)

func escape(s string) string { return html.EscapeString(s) }

type webUI struct {
	cfgPath  string
	provider aiprovider.Provider
}

func renderPage(m *Model, flash, errMsg, openKey string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>board-client 配置</title>
<style>
body{font:16px/1.4 system-ui,sans-serif;max-width:56rem;margin:2rem auto;padding:0 1rem;background:#0b1020;color:#e8eefc}
input,button,select,textarea{font:inherit;padding:.35rem .5rem;border-radius:6px;border:1px solid #334}
input[type=text],input[type=password],textarea,select{background:#11182c;color:#e8eefc;width:100%;box-sizing:border-box}
textarea{min-height:5rem}
label{display:block;margin:.8rem 0 .2rem}
table{width:100%;border-collapse:collapse;margin-top:.5rem}
th,td{border-bottom:1px solid #223;padding:.3rem;text-align:left;vertical-align:top}
button{background:#3b6cff;color:#fff;border:0;cursor:pointer;margin:.4rem .4rem 0 0}
button.secondary{background:#334}
.ok{color:#6ee7b7}.err{color:#fca5a5}
.feat{margin:.25rem 0}.sub{margin-left:1.6rem}
.new{color:#fbbf24;font-size:.85em;margin-left:.4rem}
h2{margin-top:1.6rem;font-size:1.1rem;color:#9db4ff}
details.probe{border:1px solid #223;border-radius:8px;padding:.6rem .8rem;margin:.6rem 0;background:#10182c}
details.probe summary{cursor:pointer;font-weight:600}
.preview{white-space:pre-wrap;background:#0b1020;padding:.6rem;border-radius:6px;font:13px/1.4 ui-monospace,monospace;margin-top:.5rem}
.row{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
.hint{color:#9db4ff;font-size:.9em}
</style>
</head>
<body>
<h1>本机 board-client 配置</h1>
<p>只改这台机器的 YAML，不是看板网站。key / 上报 URL 从现有文件继承。保存后 overlay 写入并 reload。</p>
`)
	if flash != "" {
		b.WriteString(`<p class="ok">` + escape(flash) + `</p>`)
	}
	if errMsg != "" {
		b.WriteString(`<p class="err">` + escape(errMsg) + `</p>`)
	}
	b.WriteString(`<form method="post" action="/save">
<h2>身份</h2>
<label>server.url</label>
<input type="text" name="url" value="` + escape(m.URL) + `">
<label>machine.key</label>
<input type="text" name="key" value="` + escape(m.Key) + `">
<label>display_name</label>
<input type="text" name="name" value="` + escape(m.Name) + `">
<label>server.machine_token（空则保留）</label>
<input type="password" name="token" value="" placeholder="` + escape(maskToken(m.Token)) + `" autocomplete="off">
`)
	var last string
	for _, f := range config.Catalog() {
		if f.Group != last {
			b.WriteString(`<h2>默认功能 · ` + escape(f.Group) + `</h2>`)
			last = f.Group
		}
		checked := ""
		if m.Enabled[f.ID] {
			checked = " checked"
		}
		badge := ""
		if isNew(m, f.ID) {
			badge = `<span class="new">新增</span>`
		}
		b.WriteString(`<div class="feat"><label><input type="checkbox" name="feat" value="` + escape(f.ID) + `"` + checked + `> ` + escape(f.Title) + badge + `</label></div>`)
		for _, s := range f.Subs {
			sc := ""
			if m.Subs[f.ID][s.ID] {
				sc = " checked"
			}
			sb := ""
			if isNew(m, f.ID+"."+s.ID) {
				sb = `<span class="new">新增</span>`
			}
			b.WriteString(`<div class="sub"><label><input type="checkbox" name="sub.` + escape(f.ID) + `" value="` + escape(s.ID) + `"` + sc + `> ` + escape(s.Title) + sb + `</label></div>`)
		}
	}
	b.WriteString(`<h2>自然语言扩展</h2>
<p class="hint">展开一条，填写描述后 Build 并预览；效果不好再补充。Build 成功后才能启用。编译需要启用 AI，并在本机设置 CURSOR_API_KEY。直接改 YAML 请用自定义扩展（见 skills/board-client-extension），不要手写 intent。</p>
`)
	if len(m.Probes) == 0 {
		b.WriteString(`<p class="hint">还没有条目。点下面添加一条。</p>`)
	}
	for i, p := range m.Probes {
		b.WriteString(renderProbeCard(m, i, p, openKey))
	}
	b.WriteString(`<button type="submit" formaction="/add-probe" class="secondary" name="probe_action" value="add">添加一条自然语言扩展</button>
<h2>自定义 · http.targets</h2>
<p class="hint">手写 HTTP 探测。Agent 请按 skills/board-client-extension 添加，不要写 status_probes.intent。</p>
<table><tr><th>service_key</th><th>name</th><th>url</th></tr>`)
	ht := m.HTTP
	for len(ht) < 3 {
		ht = append(ht, config.HTTPTarget{})
	}
	for _, t := range ht {
		b.WriteString(`<tr>
<td><input type="text" name="http_key" value="` + escape(t.ServiceKey) + `"></td>
<td><input type="text" name="http_name" value="` + escape(t.Name) + `"></td>
<td><input type="text" name="http_url" value="` + escape(t.URL) + `"></td>
</tr>`)
	}
	b.WriteString(`</table>
<h2>自定义 · probes.scripts</h2>
<p class="hint">手写脚本建议放 <code>extensions/custom/&lt;key&gt;/probe.sh</code>。</p>
<table><tr><th>service_key</th><th>name</th><th>command</th></tr>`)
	sc := m.Scripts
	for len(sc) < 3 {
		sc = append(sc, config.ProbeScript{})
	}
	for _, s := range sc {
		b.WriteString(`<tr>
<td><input type="text" name="script_key" value="` + escape(s.ServiceKey) + `"></td>
<td><input type="text" name="script_name" value="` + escape(s.Name) + `"></td>
<td><input type="text" name="script_cmd" value="` + escape(strings.Join(s.Command, " ")) + `"></td>
</tr>`)
	}
	b.WriteString(`</table>
<button type="submit">保存并 reload</button>
</form>
</body></html>`)
	return b.String()
}

func renderProbeCard(m *Model, i int, p config.StatusProbe, openKey string) string {
	kind := p.Kind
	if kind == "" {
		kind = config.StatusProbeMetric
	}
	built := m.probeBuilt(p)
	builtLabel := "未 build"
	if built {
		builtLabel = "已 build"
	}
	enLabel := "停用"
	if built && p.IsEnabled() {
		enLabel = "启用"
	}
	open := ""
	if openKey != "" && openKey == p.Key {
		open = " open"
	}
	idx := strconv.Itoa(i)
	enableDisabled := ""
	enableChecked := ""
	if !built {
		enableDisabled = " disabled"
	} else if p.IsEnabled() {
		enableChecked = " checked"
	}
	cmd := strings.Join(p.Command, " ")
	var b strings.Builder
	title := p.Key
	if title == "" {
		title = "(新条目)"
	}
	b.WriteString(`<details class="probe"` + open + `><summary>` + escape(title) + ` · ` + escape(kind) + ` · ` + builtLabel + ` · ` + enLabel + `</summary>
<input type="hidden" name="probe_key" value="` + escape(p.Key) + `">
<input type="hidden" name="probe_dir" value="` + escape(p.Dir) + `">
<input type="hidden" name="probe_command" value="` + escape(cmd) + `">
<input type="hidden" name="probe_history" value="` + escape(strings.Join(p.IntentHistory, "\n")) + `">
<div class="row">
<div><label>key</label><input type="text" name="probe_key_edit" value="` + escape(p.Key) + `"></div>
<div><label>类型</label><select name="probe_kind">` + kindOptions(kind) + `</select></div>
</div>
<label>名称</label>
<input type="text" name="probe_name" value="` + escape(p.Name) + `">
<label>自然语言描述</label>
<textarea name="probe_intent">` + escape(p.Intent) + `</textarea>
<div class="row">
<div><label>path（可选绝对路径）</label><input type="text" name="probe_path" value="` + escape(p.Path) + `"></div>
<div><label>interval</label><input type="text" name="probe_interval" value="` + escape(config.FormatDuration(p.Interval)) + `"></div>
</div>
<label>ttl_seconds（service/http）</label>
<input type="number" min="0" name="probe_ttl" value="` + formatInt(p.TTLSeconds) + `">
<label><input type="checkbox" name="probe_enable" value="` + escape(p.Key) + `"` + enableChecked + enableDisabled + `> 启用（需先 Build）</label>
<label>补充描述（追加到现有 intent）</label>
<textarea name="probe_extra"></textarea>
<button type="submit" formaction="/build" name="probe_index" value="` + idx + `">Build 并预览</button>
<button type="submit" formaction="/supplement" class="secondary" name="probe_index" value="` + idx + `">追加补充</button>
`)
	if text := m.Previews[p.Key]; text != "" {
		b.WriteString(`<div class="preview">` + escape(text) + `</div>`)
	}
	b.WriteString(`</details>`)
	return b.String()
}

func newMux(cfgPath string) http.Handler {
	return newWeb(cfgPath, nil)
}

func newWeb(cfgPath string, provider aiprovider.Provider) http.Handler {
	s := &webUI{cfgPath: cfgPath, provider: provider}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.get)
	mux.HandleFunc("/save", s.save)
	mux.HandleFunc("/build", s.build)
	mux.HandleFunc("/supplement", s.supplement)
	mux.HandleFunc("/add-probe", s.addProbe)
	return mux
}

func (s *webUI) get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m, err := loadModel(s.cfgPath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		empty := &Model{Enabled: map[string]bool{}, Subs: map[string]map[string]bool{}, Unseen: map[string]bool{}, Previews: map[string]string{}}
		_, _ = w.Write([]byte(renderPage(empty, "", err.Error(), "")))
		return
	}
	s.attachProvider(m)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "", "", "")))
}

func (s *webUI) attachProvider(m *Model) {
	if s.provider != nil {
		m.Provider = s.provider
	}
}

func (s *webUI) parseAndLoad(w http.ResponseWriter, r *http.Request) *Model {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return nil
	}
	m, err := loadModel(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return nil
	}
	s.attachProvider(m)
	applyIdentityAndFeatures(m, r)
	m.Probes = parseProbeForm(r, m)
	m.HTTP = parseHTTPForm(r)
	m.Scripts = parseScriptForm(r)
	m.TouchP, m.TouchH, m.TouchS = true, true, true
	return m
}

func applyIdentityAndFeatures(m *Model, r *http.Request) {
	if u := strings.TrimSpace(r.Form.Get("url")); u != "" {
		m.URL = u
	}
	if k := strings.TrimSpace(r.Form.Get("key")); k != "" {
		m.Key = k
	}
	m.Name = strings.TrimSpace(r.Form.Get("name"))
	if tok := strings.TrimSpace(r.Form.Get("token")); tok != "" {
		m.Token = tok
		m.tokenTouched = true
	}
	checked := map[string]bool{}
	for _, id := range r.Form["feat"] {
		checked[id] = true
	}
	for _, f := range config.Catalog() {
		m.Enabled[f.ID] = checked[f.ID]
		if len(f.Subs) == 0 {
			continue
		}
		if m.Subs[f.ID] == nil {
			m.Subs[f.ID] = map[string]bool{}
		}
		want := map[string]bool{}
		for _, id := range r.Form["sub."+f.ID] {
			want[id] = true
		}
		for _, s := range f.Subs {
			m.Subs[f.ID][s.ID] = want[s.ID]
		}
	}
}

func (s *webUI) save(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	if err := SaveAndReload(s.cfgPath, m.edit()); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderPage(m, "", err.Error(), "")))
		return
	}
	fresh, _ := loadModel(s.cfgPath)
	if fresh == nil {
		fresh = m
	}
	s.attachProvider(fresh)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(fresh, "已保存", "", "")))
}

func (s *webUI) build(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(r.Form.Get("probe_index")))
	prev, err := m.buildAt(idx)
	flash, errMsg, open := "", "", ""
	if idx >= 0 && idx < len(m.Probes) {
		open = m.Probes[idx].Key
	}
	if err != nil {
		errMsg = "Build 失败: " + err.Error()
	} else {
		flash = "Build 完成"
		if prev.Output != "" {
			flash += "，见下方预览"
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, flash, errMsg, open)))
}

func (s *webUI) supplement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(r.Form.Get("probe_index")))
	extras := r.Form["probe_extra"]
	extra := ""
	if idx >= 0 && idx < len(extras) {
		extra = extras[idx]
	}
	open := ""
	if err := m.supplementAt(idx, extra); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderPage(m, "", err.Error(), open)))
		return
	}
	if idx >= 0 && idx < len(m.Probes) {
		open = m.Probes[idx].Key
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已追加补充，请再 Build", "", open)))
}

func (s *webUI) addProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	m.Probes = append(m.Probes, config.StatusProbe{Kind: config.StatusProbeMetric, Enabled: config.BoolPtr(false)})
	m.TouchP = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已添加空条目，展开后填写并 Build", "", "")))
}

func parseProbeForm(r *http.Request, m *Model) []config.StatusProbe {
	keys := r.Form["probe_key"]
	edits := r.Form["probe_key_edit"]
	kinds := r.Form["probe_kind"]
	names := r.Form["probe_name"]
	intents := r.Form["probe_intent"]
	paths := r.Form["probe_path"]
	intervals := r.Form["probe_interval"]
	ttls := r.Form["probe_ttl"]
	dirs := r.Form["probe_dir"]
	cmds := r.Form["probe_command"]
	histories := r.Form["probe_history"]
	enabledKeys := map[string]bool{}
	for _, k := range r.Form["probe_enable"] {
		enabledKeys[strings.TrimSpace(k)] = true
	}
	origByKey := map[string]config.StatusProbe{}
	for _, p := range m.Probes {
		origByKey[p.Key] = p
	}
	var out []config.StatusProbe
	for i, orig := range keys {
		k := strings.TrimSpace(orig)
		if i < len(edits) && strings.TrimSpace(edits[i]) != "" {
			k = strings.TrimSpace(edits[i])
		}
		if k == "" {
			continue
		}
		p := config.StatusProbe{Key: k}
		if i < len(kinds) {
			p.Kind = strings.TrimSpace(kinds[i])
		}
		if i < len(names) {
			p.Name = strings.TrimSpace(names[i])
		}
		if i < len(intents) {
			p.Intent = strings.TrimSpace(intents[i])
		}
		if i < len(paths) {
			p.Path = strings.TrimSpace(paths[i])
		}
		if i < len(dirs) {
			p.Dir = strings.TrimSpace(dirs[i])
		}
		if i < len(cmds) && strings.TrimSpace(cmds[i]) != "" {
			p.Command = strings.Fields(cmds[i])
		}
		if i < len(histories) && strings.TrimSpace(histories[i]) != "" {
			p.IntentHistory = splitHistory(histories[i])
		}
		if i < len(intervals) && strings.TrimSpace(intervals[i]) != "" {
			d, err := time.ParseDuration(strings.TrimSpace(intervals[i]))
			if err == nil {
				p.Interval.Duration = d
			}
		}
		if i < len(ttls) {
			p.TTLSeconds, _ = strconv.Atoi(strings.TrimSpace(ttls[i]))
		}
		if m.probeBuilt(p) {
			p.Enabled = config.BoolPtr(enabledKeys[k])
		} else if prev, ok := origByKey[k]; ok {
			p.Enabled = prev.Enabled
		} else {
			p.Enabled = config.BoolPtr(false)
		}
		out = append(out, p)
	}
	return out
}

func splitHistory(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func kindOptions(selected string) string {
	if selected == "" {
		selected = config.StatusProbeMetric
	}
	var b strings.Builder
	for _, kind := range []string{config.StatusProbeMetric, config.StatusProbeService, config.StatusProbeHTTP} {
		sel := ""
		if selected == kind {
			sel = ` selected`
		}
		b.WriteString(`<option value="` + kind + `"` + sel + `>` + kind + `</option>`)
	}
	return b.String()
}

func formatInt(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func parseHTTPForm(r *http.Request) []config.HTTPTarget {
	keys := r.Form["http_key"]
	names := r.Form["http_name"]
	urls := r.Form["http_url"]
	var out []config.HTTPTarget
	for i := range keys {
		k := strings.TrimSpace(keys[i])
		u := ""
		if i < len(urls) {
			u = strings.TrimSpace(urls[i])
		}
		if k == "" && u == "" {
			continue
		}
		t := config.HTTPTarget{ServiceKey: k, URL: u}
		if i < len(names) {
			t.Name = strings.TrimSpace(names[i])
		}
		out = append(out, t)
	}
	return out
}

func parseScriptForm(r *http.Request) []config.ProbeScript {
	keys := r.Form["script_key"]
	names := r.Form["script_name"]
	cmds := r.Form["script_cmd"]
	var out []config.ProbeScript
	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		s := config.ProbeScript{ServiceKey: k, Format: "json"}
		if i < len(names) {
			s.Name = strings.TrimSpace(names[i])
		}
		if i < len(cmds) {
			s.Command = strings.Fields(cmds[i])
		}
		out = append(out, s)
	}
	return out
}

// RunWeb serves a loopback-only config form.
func RunWeb(ctx context.Context, cfgPath, listen string) error {
	if cfgPath == "" {
		return fmt.Errorf("--config is required")
	}
	if listen == "" {
		listen = "127.0.0.1:7439"
	}
	if err := checkLoopback(listen); err != nil {
		return err
	}
	s := &http.Server{Addr: listen, Handler: newMux(cfgPath), ReadHeaderTimeout: 4 * time.Second}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Shutdown(c)
	}()
	fmt.Fprintf(os.Stderr, "config web http://%s  (loopback only)\n", ln.Addr())
	err = s.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
