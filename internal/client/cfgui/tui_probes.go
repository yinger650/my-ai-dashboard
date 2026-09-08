package cfgui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"agentboard/internal/client/config"
)

func runEditProbes(m *Model, br *bufio.Reader, out io.Writer) error {
	for {
		fmt.Fprintln(out, "metric=机器指标  service=虚拟服务  http=HTTP 健康检查")
		fmt.Fprintln(out, "先填 key / 类型 / 名称 / 采集间隔 / 过期时间，再输入想法后 Build。完成后输入 p 预览。编译需要 ai.enabled=true，并在本机设置 CURSOR_API_KEY")
		fmt.Fprintln(out, "编号展开  a 添加  d <n> 删除  t <n> 启停  回车返回")
		if len(m.Probes) == 0 {
			fmt.Fprintln(out, "  （空）")
		}
		for i, p := range m.Probes {
			kind := p.Kind
			if kind == "" {
				kind = config.StatusProbeMetric
			}
			built := "未build"
			if m.probeBuilt(p) {
				built = "已build"
			}
			en := "停用"
			if p.IsEnabled() && m.probeBuilt(p) {
				en = "启用"
			}
			fmt.Fprintf(out, "  [%d] %s  kind=%s name=%q %s %s\n", i, p.Key, kind, p.Name, built, en)
		}
		fmt.Fprintf(out, "> ")
		line, err := br.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			return nil
		case line == "a":
			if err := addProbe(m, br, out); err != nil {
				return err
			}
		case strings.HasPrefix(line, "d"):
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "d")))
			if n < 0 || n >= len(m.Probes) {
				fmt.Fprintln(out, "编号无效")
				continue
			}
			m.TouchP = true
			m.Probes = append(m.Probes[:n], m.Probes[n+1:]...)
		case strings.HasPrefix(line, "t"):
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "t")))
			if err := m.toggleEnableAt(n); err != nil {
				fmt.Fprintln(out, err.Error())
			}
		default:
			n, convErr := strconv.Atoi(line)
			if convErr != nil || n < 0 || n >= len(m.Probes) {
				fmt.Fprintln(out, "未知命令")
				continue
			}
			if err := probeDetail(m, n, br, out); err != nil {
				return err
			}
		}
	}
}

func addProbe(m *Model, br *bufio.Reader, out io.Writer) error {
	p := config.StatusProbe{Kind: config.StatusProbeMetric, Enabled: config.BoolPtr(false)}
	fmt.Fprintf(out, "key: ")
	p.Key, _ = readTrim(br)
	fmt.Fprintf(out, "kind (metric/service/http，空=metric): ")
	if kind, _ := readTrim(br); kind != "" {
		p.Kind = kind
	}
	fmt.Fprintf(out, "name (空=key): ")
	p.Name, _ = readTrim(br)
	fmt.Fprintf(out, "采集间隔 (如 60s, 空=默认): ")
	raw, _ := readTrim(br)
	if raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			fmt.Fprintf(out, "采集间隔无效: %v\n", err)
			return nil
		}
		p.Interval.Duration = d
	}
	fmt.Fprintf(out, "过期时间秒 (service/http 可选，空=180): ")
	raw, _ = readTrim(br)
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fmt.Fprintln(out, "过期时间无效")
			return nil
		}
		p.TTLSeconds = n
	}
	fmt.Fprintf(out, "路径 (可选绝对路径): ")
	p.Path, _ = readTrim(br)
	fmt.Fprintf(out, "请输入你的想法: ")
	p.Idea, _ = readTrim(br)
	m.TouchP = true
	m.Probes = append(m.Probes, p)
	fmt.Fprintln(out, "已添加（未启用）。输入编号展开后 Build，再 p 预览。")
	return nil
}

func probeDetail(m *Model, idx int, br *bufio.Reader, out io.Writer) error {
	for {
		if idx < 0 || idx >= len(m.Probes) {
			return nil
		}
		p := m.Probes[idx]
		kind := p.Kind
		if kind == "" {
			kind = config.StatusProbeMetric
		}
		built := m.probeBuilt(p)
		fmt.Fprintf(out, "\n[%d] %s  kind=%s\n", idx, p.Key, kind)
		fmt.Fprintf(out, "名称: %s\n", p.Name)
		fmt.Fprintf(out, "启用: %v  已build: %v\n", p.IsEnabled() && built, built)
		fmt.Fprintf(out, "dir: %s\n", p.Dir)
		fmt.Fprintln(out, "自然语言描述:")
		if strings.TrimSpace(p.Intent) == "" {
			fmt.Fprintln(out, "  （Build 后生成完整规格）")
		} else {
			fmt.Fprintln(out, p.Intent)
		}
		if strings.TrimSpace(p.Idea) != "" {
			fmt.Fprintf(out, "待处理想法: %s\n", p.Idea)
		}
		if len(p.IntentHistory) > 0 {
			fmt.Fprintf(out, "历史 %d 条\n", len(p.IntentHistory))
		}
		if text := m.Previews[p.Key]; text != "" {
			fmt.Fprintln(out, "最近预览（输入 p 查看）已缓存")
		}
		fmt.Fprintln(out, "b Build  + 输入想法  p 预览  e 启停  n 改名称  回车返回")
		fmt.Fprintf(out, "> ")
		line, err := br.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		switch line {
		case "", "q", "Q":
			return nil
		case "b", "B":
			fmt.Fprintln(out, "正在 Build…")
			_, err := m.buildAt(idx)
			if err != nil {
				fmt.Fprintf(out, "Build 失败: %v\n", err)
				continue
			}
			fmt.Fprintln(out, "Build 完成。输入 p 查看返回值。")
			fmt.Fprintln(out, "自然语言描述:")
			fmt.Fprintln(out, m.Probes[idx].Intent)
		case "p", "P":
			fmt.Fprintln(out, "预览:")
			prev, err := m.previewAt(idx)
			if err != nil {
				fmt.Fprintf(out, "预览失败: %v\n", err)
				if prev.Output != "" {
					fmt.Fprintln(out, prev.Output)
				}
				continue
			}
			fmt.Fprintln(out, previewText(prev))
		case "+", "s", "S":
			fmt.Fprintf(out, "请输入你的想法: ")
			extra, _ := readTrim(br)
			if strings.TrimSpace(extra) == "" {
				fmt.Fprintln(out, "想法为空")
				continue
			}
			m.Probes[idx].Idea = strings.TrimSpace(extra)
			m.TouchP = true
			fmt.Fprintln(out, "已记下想法。请再 Build。")
		case "e", "E":
			if err := m.toggleEnableAt(idx); err != nil {
				fmt.Fprintln(out, err.Error())
			}
		case "n", "N":
			fmt.Fprintf(out, "name: ")
			if v, _ := readTrim(br); v != "" {
				m.Probes[idx].Name = v
				m.TouchP = true
			}
		case "i", "I":
			fmt.Fprintln(out, "自然语言描述只读，请用「请输入你的想法」后 Build。")
		default:
			fmt.Fprintln(out, "未知命令")
		}
	}
}
