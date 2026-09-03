// Package cfgui is the local (loopback / terminal) editor for client.yaml.
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

// RunTUI edits cfgPath interactively.
func RunTUI(cfgPath string, in io.Reader, out io.Writer) error {
	if cfgPath == "" {
		return fmt.Errorf("--config is required")
	}
	c, err := config.Read(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if c == nil {
		c = &config.Config{}
		c.Version = 1
		c.Server.URL = "https://board.yinger650.com"
		c.Machine.Key = "home-server"
	}
	br := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "\nboard-client 配置  %s\n", cfgPath)
		fmt.Fprintf(out, "  1) server.url        %s\n", c.Server.URL)
		fmt.Fprintf(out, "  2) machine_token     %s\n", maskToken(c.Server.MachineToken))
		fmt.Fprintf(out, "  3) status_probes     %d 条\n", len(c.Machine.StatusProbes))
		for i, p := range c.Machine.StatusProbes {
			fmt.Fprintf(out, "      [%d] %s  intent=%q path=%s interval=%s\n", i, p.Key, p.Intent, p.Path, p.Interval.Duration)
		}
		fmt.Fprintf(out, "  s) 保存并 reload\n  q) 退出\n> ")
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		line = strings.TrimSpace(line)
		switch line {
		case "1":
			fmt.Fprintf(out, "URL: ")
			v, _ := br.ReadString('\n')
			if v = strings.TrimSpace(v); v != "" {
				c.Server.URL = v
			}
		case "2":
			fmt.Fprintf(out, "machine_token (空则保留): ")
			var tok string
			if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
				b, err := term.ReadPassword(int(f.Fd()))
				fmt.Fprintln(out)
				if err == nil {
					tok = string(b)
				}
			} else {
				v, _ := br.ReadString('\n')
				tok = strings.TrimSpace(v)
			}
			if strings.TrimSpace(tok) != "" {
				c.Server.MachineToken = strings.TrimSpace(tok)
			}
		case "3":
			if err := editProbes(c, br, out); err != nil {
				return err
			}
		case "s", "S":
			if err := SaveAndReload(cfgPath, c); err != nil {
				fmt.Fprintf(out, "保存失败: %v\n", err)
				continue
			}
			fmt.Fprintln(out, "已写入。daemon 若在跑会 reload。")
		case "q", "Q":
			return nil
		default:
			fmt.Fprintln(out, "未知命令")
		}
	}
}

func editProbes(c *config.Config, br *bufio.Reader, out io.Writer) error {
	fmt.Fprintln(out, "a 添加  d <n> 删除  回车返回")
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
		c.Machine.StatusProbes = append(c.Machine.StatusProbes, p)
	case strings.HasPrefix(line, "d"):
		n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "d")))
		if n < 0 || n >= len(c.Machine.StatusProbes) {
			fmt.Fprintln(out, "编号无效")
			return nil
		}
		c.Machine.StatusProbes = append(c.Machine.StatusProbes[:n], c.Machine.StatusProbes[n+1:]...)
	}
	return nil
}

func readTrim(br *bufio.Reader) (string, error) {
	s, err := br.ReadString('\n')
	return strings.TrimSpace(s), err
}

func maskToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "abp_m_REPLACE_ME" {
		return "(空)"
	}
	if len(s) <= 10 {
		return "****"
	}
	return s[:8] + "…"
}

// SaveAndReload writes yaml and asks the running daemon to reload.
func SaveAndReload(cfgPath string, c *config.Config) error {
	if c.Version == 0 {
		c.Version = 1
	}
	if err := config.AtomicWrite(cfgPath, c); err != nil {
		return err
	}
	sock := c.ControlSockPath()
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
