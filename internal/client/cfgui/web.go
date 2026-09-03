package cfgui

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"agentboard/internal/client/config"
)

const page = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>board-client 配置</title>
<style>
body{font:16px/1.4 system-ui,sans-serif;max-width:52rem;margin:2rem auto;padding:0 1rem;background:#0b1020;color:#e8eefc}
input,button{font:inherit;padding:.35rem .5rem;border-radius:6px;border:1px solid #334}
input{background:#11182c;color:#e8eefc;width:100%;box-sizing:border-box}
label{display:block;margin:.8rem 0 .2rem}
table{width:100%;border-collapse:collapse;margin-top:1rem}
th,td{border-bottom:1px solid #223;padding:.3rem;text-align:left}
button{background:#3b6cff;color:#fff;border:0;cursor:pointer;margin-top:1rem}
.ok{color:#6ee7b7}.err{color:#fca5a5}
</style>
</head>
<body>
<h1>本机 board-client 配置</h1>
<p>只改这台机器的 YAML，不是看板网站。保存后经 control.sock reload。</p>
{{if .Flash}}<p class="ok">{{.Flash}}</p>{{end}}
{{if .Error}}<p class="err">{{.Error}}</p>{{end}}
<form method="post" action="/save">
<label>server.url</label>
<input name="url" value="{{.URL}}">
<label>server.machine_token</label>
<input name="token" type="password" value="{{.Token}}" placeholder="留空则保留原值" autocomplete="off">
<h2>status_probes</h2>
<table>
<tr><th>key</th><th>intent</th><th>path</th><th>interval</th></tr>
{{range .Probes}}
<tr>
<td><input name="key" value="{{.Key}}"></td>
<td><input name="intent" value="{{.Intent}}"></td>
<td><input name="path" value="{{.Path}}"></td>
<td><input name="interval" value="{{.Interval}}"></td>
</tr>
{{end}}
</table>
<button type="submit">保存并 reload</button>
</form>
</body></html>`

type view struct {
	URL    string
	Token  string
	Probes []probeView
	Flash  string
	Error  string
}

type probeView struct {
	Key, Intent, Path, Interval string
}

var pageT = template.Must(template.New("cfg").Parse(page))

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
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		v, err := loadView(cfgPath)
		if err != nil {
			v = view{Error: err.Error(), Probes: emptyProbes()}
		}
		_ = pageT.Execute(w, v)
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
		c, err := config.Read(cfgPath)
		if err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), 400)
			return
		}
		if c == nil {
			c = &config.Config{Version: 1}
		}
		if u := strings.TrimSpace(r.Form.Get("url")); u != "" {
			c.Server.URL = u
		}
		if tok := strings.TrimSpace(r.Form.Get("token")); tok != "" {
			c.Server.MachineToken = tok
		}
		keys := r.Form["key"]
		intents := r.Form["intent"]
		paths := r.Form["path"]
		intervals := r.Form["interval"]
		var probes []config.StatusProbe
		for i := range keys {
			k := strings.TrimSpace(keys[i])
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
			probes = append(probes, p)
		}
		c.Machine.StatusProbes = probes
		v, _ := loadView(cfgPath)
		v.URL = c.Server.URL
		v.Token = c.Server.MachineToken
		v.Probes = toProbeViews(c.Machine.StatusProbes)
		if err := SaveAndReload(cfgPath, c); err != nil {
			v.Error = err.Error()
			w.WriteHeader(http.StatusBadRequest)
			_ = pageT.Execute(w, v)
			return
		}
		v.Flash = "已保存"
		_ = pageT.Execute(w, v)
	})
	s := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 4 * time.Second}
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

func loadView(path string) (view, error) {
	c, err := config.Read(path)
	if err != nil {
		return view{Probes: emptyProbes()}, err
	}
	v := view{URL: c.Server.URL, Token: c.Server.MachineToken, Probes: toProbeViews(c.Machine.StatusProbes)}
	for len(v.Probes) < 6 {
		v.Probes = append(v.Probes, probeView{})
	}
	return v, nil
}

func toProbeViews(in []config.StatusProbe) []probeView {
	var out []probeView
	for _, p := range in {
		iv := ""
		if p.Interval.Duration > 0 {
			iv = p.Interval.Duration.String()
		}
		out = append(out, probeView{Key: p.Key, Intent: p.Intent, Path: p.Path, Interval: iv})
	}
	return out
}

func emptyProbes() []probeView {
	return make([]probeView, 6)
}
