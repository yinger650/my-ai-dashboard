package cfgui

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"agentboard/internal/client/config"
	"agentboard/internal/client/control"
	"golang.org/x/term"
)

type menuItem struct {
	n      int
	kind   string // ident | feat | sub | custom
	id     string
	parent string
}

// RunTUI edits cfgPath interactively.
func RunTUI(cfgPath string, in io.Reader, out io.Writer) error {
	if cfgPath == "" {
		return fmt.Errorf("--config is required")
	}
	m, err := loadModel(cfgPath)
	if err != nil {
		return err
	}
	br := bufio.NewReader(in)
	for {
		items := printTUI(out, m)
		fmt.Fprintf(out, "  s) 保存并 reload\n  q) 退出\n  t <id> 按功能 id 勾选\n> ")
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "q" || line == "Q":
			return nil
		case line == "s" || line == "S":
			if err := SaveAndReload(cfgPath, m.edit()); err != nil {
				fmt.Fprintf(out, "保存失败: %v\n", err)
				continue
			}
			fmt.Fprintln(out, "已写入。daemon 若在跑会 reload。")
			m.Unseen = map[string]bool{}
		case strings.HasPrefix(line, "t ") || strings.HasPrefix(line, "T "):
			id := strings.TrimSpace(line[2:])
			toggleID(m, id)
		default:
			n, err := strconv.Atoi(line)
			if err != nil {
				fmt.Fprintln(out, "未知命令")
				continue
			}
			it, ok := itemByN(items, n)
			if !ok {
				fmt.Fprintln(out, "编号无效")
				continue
			}
			if err := handleItem(m, it, br, in, out); err != nil {
				return err
			}
		}
	}
}

func printTUI(out io.Writer, m *Model) []menuItem {
	fmt.Fprintf(out, "\nboard-client 配置  %s\n", m.Path)
	fmt.Fprintln(out, "身份（继承，可改）")
	n := 1
	var items []menuItem
	add := func(kind, id, parent, line string) {
		items = append(items, menuItem{n: n, kind: kind, id: id, parent: parent})
		fmt.Fprintf(out, "  %d) %s\n", n, line)
		n++
	}
	add("ident", "url", "", "server.url        "+m.URL)
	add("ident", "key", "", "machine.key       "+m.Key)
	add("ident", "name", "", "display_name      "+m.Name)
	add("ident", "token", "", "machine_token     "+maskToken(m.Token))

	var lastGroup string
	for _, f := range config.Catalog() {
		if f.Group != lastGroup {
			fmt.Fprintf(out, "默认功能 · %s\n", f.Group)
			lastGroup = f.Group
		}
		mark := checkbox(m.Enabled[f.ID])
		extra := ""
		if isNew(m, f.ID) {
			extra = "  新增"
		}
		add("feat", f.ID, "", mark+" "+f.Title+extra)
		for _, s := range f.Subs {
			sm := checkbox(m.Subs[f.ID][s.ID])
			sx := ""
			if isNew(m, f.ID+"."+s.ID) {
				sx = "  新增"
			}
			add("sub", s.ID, f.ID, "    "+sm+" "+s.Title+sx)
		}
	}
	fmt.Fprintln(out, "自定义（保留）")
	add("custom", "probes", "", fmt.Sprintf("status_probes     %d 条", len(m.Probes)))
	add("custom", "http", "", fmt.Sprintf("http.targets      %d 条", len(m.HTTP)))
	add("custom", "scripts", "", fmt.Sprintf("probes.scripts    %d 条", len(m.Scripts)))
	return items
}

func checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

func itemByN(items []menuItem, n int) (menuItem, bool) {
	for _, it := range items {
		if it.n == n {
			return it, true
		}
	}
	return menuItem{}, false
}

func handleItem(m *Model, it menuItem, br *bufio.Reader, in io.Reader, out io.Writer) error {
	switch it.kind {
	case "ident":
		return editIdent(m, it.id, br, in, out)
	case "feat":
		toggleID(m, it.id)
	case "sub":
		if m.Subs[it.parent] == nil {
			m.Subs[it.parent] = map[string]bool{}
		}
		m.Subs[it.parent][it.id] = !m.Subs[it.parent][it.id]
	case "custom":
		return editCustom(m, it.id, br, out)
	}
	return nil
}

func toggleID(m *Model, id string) {
	if _, ok := m.Enabled[id]; ok {
		m.Enabled[id] = !m.Enabled[id]
		if id == "ai.discover" && m.Enabled[id] {
			if m.Subs[id] == nil {
				m.Subs[id] = map[string]bool{}
			}
			any := false
			for _, v := range m.Subs[id] {
				if v {
					any = true
					break
				}
			}
			if !any {
				for _, s := range config.DefaultDiscoverSubs() {
					m.Subs[id][s.ID] = true
				}
			}
		}
		return
	}
	for parent, subs := range m.Subs {
		if _, ok := subs[id]; ok {
			m.Subs[parent][id] = !m.Subs[parent][id]
			return
		}
	}
}

func editIdent(m *Model, id string, br *bufio.Reader, in io.Reader, out io.Writer) error {
	switch id {
	case "url":
		fmt.Fprintf(out, "URL: ")
		v, _ := readTrim(br)
		if v != "" {
			m.URL = v
		}
	case "key":
		fmt.Fprintf(out, "machine.key: ")
		v, _ := readTrim(br)
		if v != "" {
			m.Key = v
		}
	case "name":
		fmt.Fprintf(out, "display_name: ")
		v, _ := readTrim(br)
		m.Name = v
	case "token":
		fmt.Fprintf(out, "machine_token (空则保留): ")
		var tok string
		if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(out)
			if err == nil {
				tok = string(b)
			}
		} else {
			tok, _ = readTrim(br)
		}
		if strings.TrimSpace(tok) != "" {
			m.Token = strings.TrimSpace(tok)
			m.tokenTouched = true
		}
	}
	return nil
}

func editCustom(m *Model, which string, br *bufio.Reader, out io.Writer) error {
	switch which {
	case "probes":
		m.TouchP = true
		return editProbes(m, br, out)
	case "http":
		m.TouchH = true
		return editHTTP(m, br, out)
	case "scripts":
		m.TouchS = true
		return editScripts(m, br, out)
	}
	return nil
}

func editProbes(m *Model, br *bufio.Reader, out io.Writer) error {
	fmt.Fprintln(out, "a 添加  d <n> 删除  回车返回")
	for i, p := range m.Probes {
		fmt.Fprintf(out, "  [%d] %s  intent=%q path=%s\n", i, p.Key, p.Intent, p.Path)
	}
	fmt.Fprintf(out, "> ")
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	line = strings.TrimSpace(line)
	switch {
	case line == "":
		return nil
	case line == "a":
		p := config.StatusProbe{}
		fmt.Fprintf(out, "key: ")
		p.Key, _ = readTrim(br)
		fmt.Fprintf(out, "intent: ")
		p.Intent, _ = readTrim(br)
		fmt.Fprintf(out, "path (可选): ")
		p.Path, _ = readTrim(br)
		fmt.Fprintf(out, "interval (如 60s, 空=默认): ")
		raw, _ := readTrim(br)
		if raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil {
				fmt.Fprintf(out, "interval 无效: %v\n", err)
				return nil
			}
			p.Interval.Duration = d
		}
		m.Probes = append(m.Probes, p)
	case strings.HasPrefix(line, "d"):
		n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "d")))
		if n < 0 || n >= len(m.Probes) {
			fmt.Fprintln(out, "编号无效")
			return nil
		}
		m.Probes = append(m.Probes[:n], m.Probes[n+1:]...)
	}
	return nil
}

func editHTTP(m *Model, br *bufio.Reader, out io.Writer) error {
	fmt.Fprintln(out, "a 添加  d <n> 删除  回车返回")
	for i, t := range m.HTTP {
		fmt.Fprintf(out, "  [%d] %s  %s\n", i, t.ServiceKey, t.URL)
	}
	fmt.Fprintf(out, "> ")
	line, _ := readTrim(br)
	switch {
	case line == "":
		return nil
	case line == "a":
		var t config.HTTPTarget
		fmt.Fprintf(out, "service_key: ")
		t.ServiceKey, _ = readTrim(br)
		fmt.Fprintf(out, "name: ")
		t.Name, _ = readTrim(br)
		fmt.Fprintf(out, "url: ")
		t.URL, _ = readTrim(br)
		m.HTTP = append(m.HTTP, t)
	case strings.HasPrefix(line, "d"):
		n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "d")))
		if n >= 0 && n < len(m.HTTP) {
			m.HTTP = append(m.HTTP[:n], m.HTTP[n+1:]...)
		}
	}
	return nil
}

func editScripts(m *Model, br *bufio.Reader, out io.Writer) error {
	fmt.Fprintln(out, "a 添加  d <n> 删除  回车返回")
	for i, s := range m.Scripts {
		fmt.Fprintf(out, "  [%d] %s  %v\n", i, s.ServiceKey, s.Command)
	}
	fmt.Fprintf(out, "> ")
	line, _ := readTrim(br)
	switch {
	case line == "":
		return nil
	case line == "a":
		var s config.ProbeScript
		fmt.Fprintf(out, "service_key: ")
		s.ServiceKey, _ = readTrim(br)
		fmt.Fprintf(out, "name: ")
		s.Name, _ = readTrim(br)
		fmt.Fprintf(out, "command (空格分隔，绝对路径): ")
		raw, _ := readTrim(br)
		s.Command = strings.Fields(raw)
		s.Format = "json"
		m.Scripts = append(m.Scripts, s)
	case strings.HasPrefix(line, "d"):
		n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "d")))
		if n >= 0 && n < len(m.Scripts) {
			m.Scripts = append(m.Scripts[:n], m.Scripts[n+1:]...)
		}
	}
	return nil
}

func readTrim(br *bufio.Reader) (string, error) {
	s, err := br.ReadString('\n')
	return strings.TrimSpace(s), err
}

// SaveAndReload overlays yaml and asks the running daemon to reload.
func SaveAndReload(cfgPath string, ed config.Edit) error {
	if err := config.ApplyEdit(cfgPath, ed); err != nil {
		return err
	}
	doc, cfg, _, err := config.LoadDocument(cfgPath)
	if err != nil {
		return err
	}
	root := config.DocRoot(doc)
	markCatalogSeen(config.SpoolPathFromDoc(root))
	sock := cfg.ControlSockPath()
	if cfg.Storage.SpoolPath == "" {
		cfg.Storage.SpoolPath = config.SpoolPathFromDoc(root)
		sock = cfg.ControlSockPath()
	}
	resp, err := control.Call(sock, control.Request{Op: "reload"}, 3*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "已写入 %s；daemon 未运行或 reload 失败: %v\n", cfgPath, err)
		return nil
	}
	if !resp.OK {
		return fmt.Errorf("reload: %s", resp.Error)
	}
	return nil
}

func checkLoopback(listen string) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("config web must listen on loopback, got %q", listen)
}
