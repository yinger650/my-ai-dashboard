package cfgui

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"agentboard/internal/client/config"
)

func escape(s string) string { return html.EscapeString(s) }

func renderPage(m *Model, flash, errMsg string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>board-client 配置</title>
<style>
body{font:16px/1.4 system-ui,sans-serif;max-width:56rem;margin:2rem auto;padding:0 1rem;background:#0b1020;color:#e8eefc}
input,button{font:inherit;padding:.35rem .5rem;border-radius:6px;border:1px solid #334}
input[type=text],input[type=password]{background:#11182c;color:#e8eefc;width:100%;box-sizing:border-box}
label{display:block;margin:.8rem 0 .2rem}
table{width:100%;border-collapse:collapse;margin-top:.5rem}
th,td{border-bottom:1px solid #223;padding:.3rem;text-align:left;vertical-align:top}
button{background:#3b6cff;color:#fff;border:0;cursor:pointer;margin-top:1rem}
.ok{color:#6ee7b7}.err{color:#fca5a5}
.feat{margin:.25rem 0}.sub{margin-left:1.6rem}
.new{color:#fbbf24;font-size:.85em;margin-left:.4rem}
h2{margin-top:1.6rem;font-size:1.1rem;color:#9db4ff}
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
	b.WriteString(`<h2>自定义 · status_probes</h2>
<table><tr><th>key</th><th>intent</th><th>path</th><th>interval</th></tr>`)
	rows := m.Probes
	for len(rows) < 4 {
		rows = append(rows, config.StatusProbe{})
	}
	for _, p := range rows {
		b.WriteString(`<tr>
<td><input type="text" name="probe_key" value="` + escape(p.Key) + `"></td>
<td><input type="text" name="probe_intent" value="` + escape(p.Intent) + `"></td>
<td><input type="text" name="probe_path" value="` + escape(p.Path) + `"></td>
<td><input type="text" name="probe_interval" value="` + escape(config.FormatDuration(p.Interval)) + `"></td>
</tr>`)
	}
	b.WriteString(`</table>
<h2>自定义 · http.targets</h2>
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

func newMux(cfgPath string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		m, err := loadModel(cfgPath)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(renderPage(&Model{Enabled: map[string]bool{}, Subs: map[string]map[string]bool{}, Unseen: map[string]bool{}}, "", err.Error())))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderPage(m, "", "")))
	})
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m, err := loadModel(cfgPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
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
		m.Probes = parseProbeForm(r)
		m.HTTP = parseHTTPForm(r)
		m.Scripts = parseScriptForm(r)
		m.TouchP, m.TouchH, m.TouchS = true, true, true
		if err := SaveAndReload(cfgPath, m.edit()); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(renderPage(m, "", err.Error())))
			return
		}
		fresh, _ := loadModel(cfgPath)
		if fresh == nil {
			fresh = m
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderPage(fresh, "已保存", "")))
	})
	return mux
}

func parseProbeForm(r *http.Request) []config.StatusProbe {
	keys := r.Form["probe_key"]
	intents := r.Form["probe_intent"]
	paths := r.Form["probe_path"]
	intervals := r.Form["probe_interval"]
	var out []config.StatusProbe
	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		p := config.StatusProbe{Key: k}
		if i < len(intents) {
			p.Intent = strings.TrimSpace(intents[i])
		}
		if i < len(paths) {
			p.Path = strings.TrimSpace(paths[i])
		}
		if i < len(intervals) && strings.TrimSpace(intervals[i]) != "" {
			d, err := time.ParseDuration(strings.TrimSpace(intervals[i]))
			if err == nil {
				p.Interval.Duration = d
			}
		}
		out = append(out, p)
	}
	return out
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
