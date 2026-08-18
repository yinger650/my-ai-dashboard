// Package summarize builds a short markdown digest of service / agent logs.
package summarize

import (
	"strings"
	"unicode/utf8"
)

const maxOut = 4000

// Logs produces a markdown summary of recent log bodies.
func Logs(title string, bodies []string) string {
	var b strings.Builder
	if title == "" {
		title = "日志总结"
	}
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")

	joined := strings.Join(bodies, "\n")
	errors := extractLines(joined, []string{"error", "fail", "fatal", "panic", "exception", "timed out", "timeout"})
	warns := extractLines(joined, []string{"warn", "warning", "deprecated"})

	b.WriteString("- 日志条数：")
	b.WriteString(itoa(len(bodies)))
	b.WriteString("\n")
	b.WriteString("- 错误相关行：")
	b.WriteString(itoa(len(errors)))
	b.WriteString("\n")
	b.WriteString("- 警告相关行：")
	b.WriteString(itoa(len(warns)))
	b.WriteString("\n\n")

	if len(errors) > 0 {
		b.WriteString("### 错误\n\n")
		for i, line := range capSlice(errors, 8) {
			b.WriteString(itoa(i + 1))
			b.WriteString(". ")
			b.WriteString(clip(line, 240))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(warns) > 0 {
		b.WriteString("### 警告\n\n")
		for i, line := range capSlice(warns, 5) {
			b.WriteString(itoa(i + 1))
			b.WriteString(". ")
			b.WriteString(clip(line, 240))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("### 摘录\n\n")
	excerpt := clip(strings.TrimSpace(joined), 1200)
	if excerpt == "" {
		excerpt = "（无可用日志正文）"
	}
	b.WriteString(excerpt)
	b.WriteString("\n")

	out := b.String()
	if utf8.RuneCountInString(out) > maxOut {
		r := []rune(out)
		out = string(r[:maxOut]) + "\n…"
	}
	return out
}

func extractLines(text string, needles []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		hit := false
		for _, n := range needles {
			if strings.Contains(low, n) {
				hit = true
				break
			}
		}
		if !hit || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}

func capSlice(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
