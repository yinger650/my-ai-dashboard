package cfgui

import (
	"context"
	"encoding/json"
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
textarea.spec{min-height:10rem;font:13px/1.4 ui-monospace,monospace}
label{display:block;margin:.8rem 0 .2rem}
table{width:100%;border-collapse:collapse;margin-top:.5rem}
th,td{border-bottom:1px solid #223;padding:.3rem;text-align:left;vertical-align:top}
th.op,td.op{width:4.5rem;white-space:nowrap}
button.row-del{background:#5c2b2b;margin:0}
button{background:#3b6cff;color:#fff;border:0;cursor:pointer;margin:.4rem .4rem 0 0}
button.secondary{background:#334}
button:disabled{opacity:.45;cursor:not-allowed}
.ok{color:#6ee7b7}.err{color:#fca5a5}
.feat{margin:.25rem 0}.sub{margin-left:1.6rem}
.new{color:#fbbf24;font-size:.85em;margin-left:.4rem}
h2{margin-top:1.6rem;font-size:1.1rem;color:#9db4ff}
details.probe{border:1px solid #223;border-radius:8px;padding:.6rem .8rem;margin:.6rem 0;background:#10182c}
details.probe summary{cursor:pointer;font-weight:600}
.preview{white-space:pre-wrap;background:#0b1020;padding:.6rem;border-radius:6px;font:13px/1.4 ui-monospace,monospace;margin-top:.5rem}
.preview[hidden],.build-status .state[hidden]{display:none}
.row{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
.hint{color:#9db4ff;font-size:.9em}
.actions{display:flex;align-items:center;flex-wrap:wrap;gap:.4rem;margin-top:.5rem}
.build-status{min-height:1.4rem;margin:.35rem 0 0}
textarea.spec[readonly]{background:#0d1424;color:#c5d0ea;cursor:default}
.spinner{display:inline-block;width:1.15rem;height:1.15rem;margin-left:.35rem;border:2px solid #334;border-top-color:#9db4ff;border-radius:50%;animation:spin .8s linear infinite;vertical-align:middle}
@keyframes spin{to{transform:rotate(360deg)}}
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
	b.WriteString(`<form method="post" action="save">
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
<p class="hint">展开一条，先填 key、类型、名称、采集间隔和过期时间。在「请输入你的想法」里描述需求后点 Build；按钮旁显示 building 并转圈。完成后点旁边的预览，返回值出现在按钮下方。完整探测规格只读显示在上方文本框。编译需要启用 AI，并在本机设置 CURSOR_API_KEY。直接改 YAML 请用自定义扩展（见 skills/board-client-extension）。</p>
`)
	if len(m.Probes) == 0 {
		b.WriteString(`<p class="hint">还没有条目。点下面添加一条。</p>`)
	}
	for i, p := range m.Probes {
		showPrev := flash == "预览" || strings.HasPrefix(errMsg, "预览失败")
		b.WriteString(renderProbeCard(m, i, p, openKey, showPrev))
	}
	b.WriteString(`<button type="submit" formaction="add-probe" class="secondary" name="probe_action" value="add">添加一条自然语言扩展</button>
<h2>自定义 · http.targets</h2>
<p class="hint">手写 HTTP 探测。Agent 请按 skills/board-client-extension 添加，不要写 status_probes.intent。</p>
<table><thead><tr><th>service_key</th><th>name</th><th>url</th><th class="op">操作</th></tr></thead>
<tbody id="http-body">`)
	for i, t := range m.HTTP {
		b.WriteString(httpRowHTML(i, t))
	}
	b.WriteString(`</tbody></table>
<button type="submit" formaction="add-http" class="secondary" data-add-row="http">添加一行</button>
<template id="tpl-http-row">` + httpRowHTML(0, config.HTTPTarget{}) + `</template>
<h2>自定义 · probes.scripts</h2>
<p class="hint">手写脚本建议放 <code>extensions/custom/&lt;key&gt;/probe.sh</code>。</p>
<table><thead><tr><th>service_key</th><th>name</th><th>command</th><th class="op">操作</th></tr></thead>
<tbody id="script-body">`)
	for i, s := range m.Scripts {
		b.WriteString(scriptRowHTML(i, s))
	}
	b.WriteString(`</tbody></table>
<button type="submit" formaction="add-script" class="secondary" data-add-row="script">添加一行</button>
<template id="tpl-script-row">` + scriptRowHTML(0, config.ProbeScript{}) + `</template>
<button type="submit">保存并 reload</button>
</form>
` + probeScript() + `
</body></html>`)
	return b.String()
}

func httpRowHTML(i int, t config.HTTPTarget) string {
	return `<tr>
<td><input type="text" name="http_key" value="` + escape(t.ServiceKey) + `"></td>
<td><input type="text" name="http_name" value="` + escape(t.Name) + `"></td>
<td><input type="text" name="http_url" value="` + escape(t.URL) + `"></td>
<td class="op"><button type="submit" formaction="del-http" class="secondary row-del" name="row_index" value="` + strconv.Itoa(i) + `" data-del-row>删除</button></td>
</tr>`
}

func scriptRowHTML(i int, s config.ProbeScript) string {
	return `<tr>
<td><input type="text" name="script_key" value="` + escape(s.ServiceKey) + `"></td>
<td><input type="text" name="script_name" value="` + escape(s.Name) + `"></td>
<td><input type="text" name="script_cmd" value="` + escape(strings.Join(s.Command, " ")) + `"></td>
<td class="op"><button type="submit" formaction="del-script" class="secondary row-del" name="row_index" value="` + strconv.Itoa(i) + `" data-del-row>删除</button></td>
</tr>`
}

func renderProbeCard(m *Model, i int, p config.StatusProbe, openKey string, showPreview bool) string {
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
	previewBtnDis := ""
	if !built {
		previewBtnDis = " disabled"
	}
	hiddenPrev := ""
	if text := m.Previews[p.Key]; text != "" {
		hide := " hidden"
		if showPreview && openKey == p.Key {
			hide = ""
		}
		hiddenPrev = `<div class="preview" data-preview` + hide + `>` + escape(text) + `</div>`
	} else {
		hiddenPrev = `<div class="preview" data-preview hidden></div>`
	}
	b.WriteString(`<details class="probe"` + open + ` data-idx="` + idx + `"><summary>` + escape(title) + ` · ` + escape(kind) + ` · ` + builtLabel + ` · ` + enLabel + `</summary>
<input type="hidden" name="probe_key" value="` + escape(p.Key) + `">
<input type="hidden" name="probe_dir" data-dir value="` + escape(p.Dir) + `">
<input type="hidden" name="probe_command" data-command value="` + escape(cmd) + `">
<input type="hidden" name="probe_history" data-history value="` + escape(strings.Join(p.IntentHistory, "\n")) + `">
<input type="hidden" name="probe_intent" data-intent value="` + escape(p.Intent) + `">
<div class="row">
<div><label>key</label><input type="text" name="probe_key_edit" value="` + escape(p.Key) + `"></div>
<div><label>类型</label><select name="probe_kind">` + kindOptions(kind) + `</select></div>
</div>
<label>名称</label>
<input type="text" name="probe_name" value="` + escape(p.Name) + `">
<div class="row">
<div><label>采集间隔</label><input type="text" name="probe_interval" value="` + escape(config.FormatDuration(p.Interval)) + `" placeholder="如 60s、1m"></div>
<div><label>过期时间（秒）</label><input type="number" min="0" name="probe_ttl" value="` + formatInt(p.TTLSeconds) + `" placeholder="service/http 默认 180"></div>
</div>
<label>路径（可选）</label>
<input type="text" name="probe_path" value="` + escape(p.Path) + `" placeholder="绝对路径，可空">
<label><input type="checkbox" name="probe_enable" value="` + escape(p.Key) + `"` + enableChecked + enableDisabled + `> 启用（需先 Build）</label>
<label>自然语言描述</label>
<textarea class="spec" readonly tabindex="-1" data-intent-view placeholder="Build 后显示完整探测规格（只读）">` + escape(p.Intent) + `</textarea>
<label>请输入你的想法</label>
<textarea name="probe_idea" data-idea placeholder="例如：统计某个目录占用，或检查本机某个 HTTP 地址">` + escape(p.Idea) + `</textarea>
<div class="actions">
<button type="submit" formaction="build" name="probe_index" value="` + idx + `" data-build>Build</button>
<button type="submit" formaction="preview" class="secondary" name="probe_index" value="` + idx + `" data-preview-btn` + previewBtnDis + `>预览</button>
</div>
<div class="build-status" data-side>
<p class="state idle" hidden>空闲</p>
<p class="state building" hidden><span>building</span><span class="spinner"></span></p>
<p class="state previewing" hidden><span>预览中</span><span class="spinner"></span></p>
<p class="state done" hidden>Build 完成，可点预览</p>
<p class="state err" hidden></p>
</div>
` + hiddenPrev + `
</details>`)
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
	mux.HandleFunc("/preview", s.preview)
	mux.HandleFunc("/supplement", s.supplement)
	mux.HandleFunc("/add-probe", s.addProbe)
	mux.HandleFunc("/add-http", s.addHTTP)
	mux.HandleFunc("/add-script", s.addScript)
	mux.HandleFunc("/del-http", s.delHTTP)
	mux.HandleFunc("/del-script", s.delScript)
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

func parseHTTP(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		return r.ParseMultipartForm(32 << 20)
	}
	return r.ParseForm()
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("format") == "json"
}

type probeActionJSON struct {
	OK      bool   `json:"ok"`
	Flash   string `json:"flash,omitempty"`
	Error   string `json:"error,omitempty"`
	Intent  string `json:"intent,omitempty"`
	History string `json:"history,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Command string `json:"command,omitempty"`
	Built   bool   `json:"built"`
	Preview string `json:"preview,omitempty"`
	Key     string `json:"key,omitempty"`
}

func writeProbeJSON(w http.ResponseWriter, status int, v probeActionJSON) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *webUI) parseAndLoad(w http.ResponseWriter, r *http.Request) *Model {
	if err := parseHTTP(r); err != nil {
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
	}
	if wantsJSON(r) {
		out := probeActionJSON{OK: err == nil, Flash: flash, Error: errMsg}
		if idx >= 0 && idx < len(m.Probes) {
			p := m.Probes[idx]
			out.Key = p.Key
			out.Intent = p.Intent
			out.History = strings.Join(p.IntentHistory, "\n")
			out.Dir = p.Dir
			out.Command = strings.Join(p.Command, " ")
			out.Built = m.probeBuilt(p)
			out.Preview = previewText(prev)
		}
		status := http.StatusOK
		if err != nil {
			status = http.StatusBadRequest
		}
		writeProbeJSON(w, status, out)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, flash, errMsg, open)))
}

func (s *webUI) preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(r.Form.Get("probe_index")))
	prev, err := m.previewAt(idx)
	flash, errMsg, open := "", "", ""
	if idx >= 0 && idx < len(m.Probes) {
		open = m.Probes[idx].Key
	}
	if err != nil {
		errMsg = "预览失败: " + err.Error()
	} else {
		flash = "预览"
	}
	if wantsJSON(r) {
		out := probeActionJSON{OK: err == nil, Flash: flash, Error: errMsg, Preview: previewText(prev)}
		if idx >= 0 && idx < len(m.Probes) {
			out.Key = m.Probes[idx].Key
			out.Built = m.probeBuilt(m.Probes[idx])
		}
		status := http.StatusOK
		if err != nil {
			status = http.StatusBadRequest
		}
		writeProbeJSON(w, status, out)
		return
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

func (s *webUI) addHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	m.HTTP = append(m.HTTP, config.HTTPTarget{})
	m.TouchH = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已添加一行 HTTP 目标", "", "")))
}

func (s *webUI) addScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	m.Scripts = append(m.Scripts, config.ProbeScript{Format: "json"})
	m.TouchS = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已添加一行脚本", "", "")))
}

func (s *webUI) delHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(r.Form.Get("row_index")))
	if idx >= 0 && idx < len(m.HTTP) {
		m.HTTP = append(m.HTTP[:idx], m.HTTP[idx+1:]...)
		m.TouchH = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已删除一行 HTTP 目标", "", "")))
}

func (s *webUI) delScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.parseAndLoad(w, r)
	if m == nil {
		return
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(r.Form.Get("row_index")))
	if idx >= 0 && idx < len(m.Scripts) {
		m.Scripts = append(m.Scripts[:idx], m.Scripts[idx+1:]...)
		m.TouchS = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPage(m, "已删除一行脚本", "", "")))
}

func parseProbeForm(r *http.Request, m *Model) []config.StatusProbe {
	keys := r.Form["probe_key"]
	edits := r.Form["probe_key_edit"]
	kinds := r.Form["probe_kind"]
	names := r.Form["probe_name"]
	intents := r.Form["probe_intent"]
	ideas := r.Form["probe_idea"]
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
		if i < len(ideas) {
			p.Idea = strings.TrimSpace(ideas[i])
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

func probeScript() string {
	return `<script>
(function(){
  function params(form, idx){
    var fd = new FormData(form);
    fd.set("probe_index", idx);
    return new URLSearchParams(fd);
  }
  function setState(side, name, errText){
    if(!side) return;
    side.querySelectorAll(".state").forEach(function(el){ el.hidden = true; });
    var el = side.querySelector(".state." + name);
    if(!el) return;
    el.hidden = false;
    if(name === "err") el.textContent = errText || "失败";
  }
  function card(btn){
    return btn.closest("details.probe");
  }
  document.addEventListener("click", function(ev){
    var add = ev.target.closest("[data-add-row]");
    if(add){
      ev.preventDefault();
      var kind = add.getAttribute("data-add-row");
      var body = document.getElementById(kind + "-body");
      var tpl = document.getElementById("tpl-" + kind + "-row");
      if(body && tpl) body.appendChild(tpl.content.cloneNode(true));
      return;
    }
    var del = ev.target.closest("[data-del-row]");
    if(del){
      ev.preventDefault();
      var tr = del.closest("tr");
      if(tr) tr.remove();
      return;
    }
    var build = ev.target.closest("[data-build]");
    var prev = ev.target.closest("[data-preview-btn]");
    if(!build && !prev) return;
    var form = ev.target.form || (ev.target.closest("form"));
    if(!form) return;
    ev.preventDefault();
    var btn = build || prev;
    var idx = btn.value;
    var box = card(btn);
    var side = box && box.querySelector("[data-side]");
    var url = build ? "build" : "preview";
    if(build) setState(side, "building");
    else setState(side, "previewing");
    fetch(url, {
      method: "POST",
      body: params(form, idx),
      headers: {"Accept":"application/json","Content-Type":"application/x-www-form-urlencoded"}
    }).then(function(r){
      return r.json().then(function(j){ return {ok: r.ok && j.ok, j: j}; });
    }).then(function(res){
      var j = res.j || {};
      if(box && j.intent != null){
        var ta = box.querySelector("[data-intent]");
        if(ta) ta.value = j.intent;
        var view = box.querySelector("[data-intent-view]");
        if(view){ view.value = j.intent; view.readOnly = true; }
      }
      if(box && j.history != null){
        var h = box.querySelector("[data-history]");
        if(h) h.value = j.history;
      }
      if(box && j.dir != null){
        var d = box.querySelector("[data-dir]");
        if(d) d.value = j.dir;
      }
      if(box && j.command != null){
        var c = box.querySelector("[data-command]");
        if(c) c.value = j.command;
      }
      if(build){
        var idea = box && box.querySelector("[data-idea]");
        if(idea && res.ok) idea.value = "";
        var pbtn = box && box.querySelector("[data-preview-btn]");
        if(pbtn) pbtn.disabled = !j.built;
        var pv = box && box.querySelector("[data-preview]");
        if(pv) pv.hidden = true;
        setState(side, res.ok ? "done" : "err", j.error || "Build 失败");
        return;
      }
      var out = box && box.querySelector("[data-preview]");
      if(!out && box){
        out = document.createElement("div");
        out.className = "preview";
        out.setAttribute("data-preview", "");
        box.appendChild(out);
      }
      if(out){
        out.hidden = false;
        out.textContent = j.preview || j.error || "";
      }
      if(!res.ok) setState(side, "err", j.error || "预览失败");
      else if(side) side.querySelectorAll(".state").forEach(function(el){ el.hidden = true; });
    }).catch(function(e){
      setState(side, "err", String(e));
    });
  });
})();
</script>`
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
