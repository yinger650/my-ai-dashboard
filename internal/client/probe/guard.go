// Package probe runs locally configured scripts and maps stdout to Board events.
package probe

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unitRe = regexp.MustCompile(`^[A-Za-z0-9@._:-]{1,128}\.(service|timer|socket)$`)

// CheckScript enforces absolute path, executable bit, and not group/other writable.
func CheckScript(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("probe command must be an absolute path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("probe path is a directory")
	}
	mode := info.Mode()
	if mode&0o111 == 0 {
		return fmt.Errorf("probe is not executable")
	}
	if mode&0o022 != 0 {
		return fmt.Errorf("probe must not be group/other writable")
	}
	return nil
}

// Expand fills {unit} / {path} placeholders after validating params.
func Expand(argv []string, params map[string]string, allowPaths []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty argv")
	}
	out := make([]string, len(argv))
	for i, a := range argv {
		s := a
		if strings.Contains(s, "{unit}") {
			unit := params["unit"]
			if !unitRe.MatchString(unit) {
				return nil, fmt.Errorf("invalid unit %q", unit)
			}
			s = strings.ReplaceAll(s, "{unit}", unit)
		}
		if strings.Contains(s, "{path}") {
			p := params["path"]
			if err := checkAllowPath(p, allowPaths); err != nil {
				return nil, err
			}
			s = strings.ReplaceAll(s, "{path}", p)
		}
		if strings.ContainsAny(s, "|&;<>`$(){}") && strings.Contains(a, "{") {
			return nil, fmt.Errorf("expanded arg contains shell metacharacters")
		}
		out[i] = s
	}
	return out, nil
}

func checkAllowPath(p string, allowPaths []string) error {
	if p == "" || !filepath.IsAbs(p) {
		return fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(p)
	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file")
	}
	if len(allowPaths) == 0 {
		return fmt.Errorf("allow_paths is required for {path}")
	}
	for _, g := range allowPaths {
		if matchPathGlob(g, clean) {
			return nil
		}
	}
	return fmt.Errorf("path %q is not in allow_paths", clean)
}

func matchPathGlob(glob, path string) bool {
	glob = filepath.Clean(glob)
	path = filepath.Clean(path)
	if strings.HasSuffix(glob, string(filepath.Separator)+"**") || strings.HasSuffix(glob, "/**") {
		prefix := strings.TrimSuffix(strings.TrimSuffix(glob, "**"), string(filepath.Separator))
		if prefix == "" {
			return true
		}
		return path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator))
	}
	ok, err := filepath.Match(glob, path)
	return err == nil && ok
}

// ValidUnit reports whether unit is a safe systemd unit name.
func ValidUnit(unit string) bool { return unitRe.MatchString(unit) }
