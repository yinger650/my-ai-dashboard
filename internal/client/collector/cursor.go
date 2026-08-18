package collector

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxTranscriptBytes = 64 * 1024

// Transcript is a discovered Cursor / agent log file.
type Transcript struct {
	Path    string
	Rel     string
	Title   string
	Text    string
	Size    int64
	ModUnix int64
}

// ScanTranscripts walks roots for agent transcript-like files.
func ScanTranscripts(roots []string) []Transcript {
	var out []Transcript
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if t, ok := readTranscript(root, filepath.Base(root)); ok && !seen[t.Path] {
				seen[t.Path] = true
				out = append(out, t)
			}
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !looksLikeTranscript(path, d.Name()) {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			t, ok := readTranscript(path, rel)
			if !ok || seen[t.Path] {
				return nil
			}
			seen[t.Path] = true
			out = append(out, t)
			return nil
		})
	}
	return out
}

func looksLikeTranscript(path, name string) bool {
	low := strings.ToLower(name)
	if low == "summary.json" || strings.HasSuffix(low, ".jsonl") {
		return true
	}
	if !pathHasTranscriptDir(path) {
		return false
	}
	switch {
	case strings.HasSuffix(low, ".json"), strings.HasSuffix(low, ".md"), strings.HasSuffix(low, ".txt"):
		return true
	}
	return false
}

func pathHasTranscriptDir(path string) bool {
	dir := filepath.ToSlash(filepath.Dir(path))
	for _, part := range strings.Split(dir, "/") {
		n := strings.ToLower(part)
		if n == "transcript" || n == "transcripts" || n == "agent-transcripts" || n == "agent_transcripts" {
			return true
		}
		if strings.HasSuffix(n, "-transcripts") || strings.HasSuffix(n, "_transcripts") {
			return true
		}
	}
	return false
}

func readTranscript(path, rel string) (Transcript, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return Transcript{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return Transcript{}, false
	}
	defer f.Close()
	buf := make([]byte, maxTranscriptBytes)
	n, _ := f.Read(buf)
	raw := buf[:n]
	text := extractTranscriptText(path, raw)
	text = clipRunes(strings.TrimSpace(text), 8000)
	if text == "" {
		return Transcript{}, false
	}
	title := filepath.Base(path)
	if rel != "" {
		title = rel
	}
	return Transcript{
		Path:    path,
		Rel:     rel,
		Title:   title,
		Text:    text,
		Size:    st.Size(),
		ModUnix: st.ModTime().Unix(),
	}, true
}

func extractTranscriptText(path string, raw []byte) string {
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".jsonl") {
		var parts []string
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var v any
			if json.Unmarshal([]byte(line), &v) == nil {
				if s := collectStrings(v, 0); s != "" {
					parts = append(parts, s)
				}
			} else {
				parts = append(parts, line)
			}
		}
		return strings.Join(parts, "\n")
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return collectStrings(v, 0)
	}
	return string(raw)
}

func collectStrings(v any, depth int) string {
	if depth > 8 {
		return ""
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" || looksLikeID(s) {
			return ""
		}
		return s
	case map[string]any:
		prefer := []string{"summary", "markdown", "text", "content", "message"}
		var parts []string
		for _, k := range prefer {
			if child, ok := t[k]; ok {
				if s := collectStrings(child, depth+1); s != "" {
					parts = append(parts, s)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
		for _, child := range t {
			if s := collectStrings(child, depth+1); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	case []any:
		var parts []string
		for _, child := range t {
			if s := collectStrings(child, depth+1); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func looksLikeID(s string) bool {
	if len(s) >= 32 && len(s) <= 64 {
		for _, c := range s {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') && c != '-' {
				return false
			}
		}
		return true
	}
	return false
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
