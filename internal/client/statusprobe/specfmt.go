package statusprobe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
)

// SpecTitle is the first heading every compiled probe spec must start with.
const SpecTitle = "# AgentBoard 探测规格"

// SpecDocument is the canonical natural-language contract for a probe.
// ExpandSpec fills this shape; any coding model should implement it in one shot.
// Keep in sync with skills/board-client-extension/SKILL.md.
const SpecDocument = SpecTitle + `

## 元数据
- key: <探针 key，[a-z0-9._-]{1,64}>
- kind: metric | service | http
- name: <看板上的中文名称>
- path: <可选，要采集的绝对路径；没有则写 无>

## 要做什么
用条目写清本机只读采集步骤：读哪些文件/命令、如何计算、失败时怎么办。
不要改系统、不要发网络（http 类型除外，且 http 不写脚本）。

## 输出
### kind = metric 或 service
脚本 stdout 必须是且仅是一个 JSON 对象，字段如下：

` + "```" + `json
{
  "state": "running",
  "summary": "一句话中文摘要",
  "severity": "normal",
  "statuses": [
    {"key": "snake_case", "label": "中文标签", "value": "字符串", "unit": ""}
  ]
}
` + "```" + `

- state：通常 running；采不到可写 failed
- severity：normal / warning / error / critical
- statuses[].key：稳定英文蛇形；label：中文；value：字符串
- metric：value 尽量是十进制数字（进入机器卡片）；不要 logs / pinned_markdown
- service：value 可以是文本；可另加 logs（滚动日志）和 pinned_markdown（钉在服务详情的 Markdown 表格或说明）

` + "```" + `json
{
  "logs": [{"markdown": "一行说明", "severity": "info"}],
  "pinned_markdown": "| 列 | 值 |\n|---|---|\n| a | b |"
}
` + "```" + `

### kind = http
不要写 shell。只输出：

` + "```" + `json
{"url":"http://127.0.0.1:8080/health","method":"GET","expect_status":[200],"expect_contains":""}
` + "```" + `

url 必须是无用户名密码的 http/https 绝对地址；method 只能 GET 或 HEAD；状态码 100–599。

## 约束
- POSIX sh，首行 #!/bin/sh；只输出脚本或上述 JSON，不要解释
- 禁止 curl / wget、读 token 环境变量、调用 /ingest/、改配置或服务
- 禁止把不可信字符串拼进 shell
- 失败时仍输出合法 JSON，severity=error，summary 说明原因
`

// RenderSpecSkeleton fills metadata from the probe and puts the user idea under 要做什么.
func RenderSpecSkeleton(p config.StatusProbe, idea string) string {
	kind := strings.TrimSpace(p.Kind)
	if kind == "" {
		kind = config.StatusProbeMetric
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = p.Key
	}
	path := strings.TrimSpace(p.Path)
	if path == "" {
		path = "无"
	}
	body := strings.TrimSpace(idea)
	if body == "" {
		body = strings.TrimSpace(p.Intent)
	}
	if body == "" {
		body = "（待补充）"
	}
	var b strings.Builder
	b.WriteString(SpecTitle)
	b.WriteString("\n\n## 元数据\n")
	fmt.Fprintf(&b, "- key: %s\n", p.Key)
	fmt.Fprintf(&b, "- kind: %s\n", kind)
	fmt.Fprintf(&b, "- name: %s\n", name)
	fmt.Fprintf(&b, "- path: %s\n", path)
	b.WriteString("\n## 要做什么\n")
	b.WriteString(body)
	b.WriteString("\n\n## 输出\n")
	switch kind {
	case config.StatusProbeHTTP:
		b.WriteString("kind=http：输出受限 HTTP JSON（url/method/expect_status/expect_contains），不要写脚本。\n")
	case config.StatusProbeService:
		b.WriteString("kind=service：stdout 一个 probe.Result JSON，可含 logs 与 pinned_markdown。\n")
	default:
		b.WriteString("kind=metric：stdout 一个 probe.Result JSON；statuses.value 尽量为数字。\n")
	}
	b.WriteString("\n## 约束\n")
	b.WriteString("- POSIX sh（http 除外）；禁止 curl/wget、token、/ingest/、改系统\n")
	b.WriteString("- 失败仍输出合法 JSON，severity=error\n")
	return b.String()
}

// LooksLikeSpec reports whether text is already a structured probe spec.
func LooksLikeSpec(text string) bool {
	s := strings.TrimSpace(text)
	return strings.HasPrefix(s, SpecTitle) || strings.Contains(s, "## 要做什么") && strings.Contains(s, "## 输出")
}

// ExtractSpec strips an optional Markdown fence around a probe spec.
func ExtractSpec(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if nl := strings.Index(text, "\n"); nl >= 0 {
			text = text[nl+1:]
		}
		if i := strings.LastIndex(text, "```"); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
	}
	return strings.TrimSpace(text)
}

// ExpandSpec turns a user idea (plus optional current spec) into a full probe spec.
// On AI failure it returns RenderSpecSkeleton so the UI still has a complete document.
func (c *Compiler) ExpandSpec(ctx context.Context, p config.StatusProbe, idea string) (string, error) {
	idea = strings.TrimSpace(idea)
	if idea == "" && strings.TrimSpace(p.Intent) == "" {
		return "", fmt.Errorf("empty idea")
	}
	if !c.AIEnabled || c.Provider == nil {
		return RenderSpecSkeleton(p, idea), nil
	}
	untrusted := "key=" + p.Key + "\nkind=" + p.Kind + "\nname=" + p.Name
	if p.Path != "" {
		untrusted += "\npath=" + p.Path
	}
	if cur := strings.TrimSpace(p.Intent); cur != "" {
		untrusted += "\ncurrent_spec=\n" + cur
	}
	if idea != "" {
		untrusted += "\nidea=" + idea
	}
	res, err := c.Provider.Run(ctx, aiprovider.Request{
		Task:       "probe_spec",
		UserPrompt: SpecDocument,
		Untrusted:  untrusted,
		Timeout:    90 * time.Second,
		MaxRunes:   5000,
	})
	if err != nil {
		return RenderSpecSkeleton(p, idea), nil
	}
	spec := ExtractSpec(res.Text)
	if spec == "" {
		return RenderSpecSkeleton(p, idea), nil
	}
	if !strings.HasPrefix(strings.TrimSpace(spec), "#") {
		spec = RenderSpecSkeleton(p, spec)
	}
	return spec, nil
}

// ApplyIdea writes the previous spec into history and replaces Intent with spec.
func ApplyIdea(p *config.StatusProbe, spec string) {
	if p == nil {
		return
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return
	}
	if cur := strings.TrimSpace(p.Intent); cur != "" && cur != spec {
		p.IntentHistory = append(p.IntentHistory, cur)
	}
	p.Intent = spec
	p.Idea = ""
}
