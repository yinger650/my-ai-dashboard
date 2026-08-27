package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"agentboard/internal/client/hostsnap"
)

// ReadNginx walks allowlisted config roots and extracts reverse-proxy locations.
func ReadNginx(roots []string, hostRoot string) *hostsnap.Nginx {
	if len(roots) == 0 {
		roots = []string{"/etc/nginx", "/www/server/nginx/conf"}
	}
	var files []string
	for _, r := range roots {
		files = append(files, listNginxFiles(joinRoot(hostRoot, r), joinRoot(hostRoot, r))...)
	}
	seen := map[string][]byte{}
	var proxies []hostsnap.Proxy
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		seen[f] = data
	}
	for _, extra := range collectNginxIncludes(seen, hostRoot) {
		if _, ok := seen[extra]; ok {
			continue
		}
		data, err := os.ReadFile(extra)
		if err != nil {
			continue
		}
		seen[extra] = data
	}

	// Parse each file independently; includes already expanded via directory walk.
	upstreams := map[string]string{}
	for _, data := range seen {
		for k, v := range ParseNginxUpstreams(string(data)) {
			upstreams[k] = v
		}
	}
	for _, data := range seen {
		proxies = append(proxies, ParseNginxProxies(string(data), upstreams)...)
	}
	return &hostsnap.Nginx{Available: len(seen) > 0, Proxies: dedupeProxies(proxies)}
}

func listNginxFiles(dir, allowPrefix string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		// maybe a single file
		if st, err2 := os.Stat(dir); err2 == nil && !st.IsDir() {
			return []string{dir}
		}
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		p := filepath.Join(dir, name)
		if !strings.HasPrefix(p, allowPrefix) {
			continue
		}
		if e.IsDir() {
			if name == "sites-available" {
				continue // only loaded trees (sites-enabled, conf.d, vhost)
			}
			out = append(out, listNginxFiles(p, allowPrefix)...)
			continue
		}
		if strings.HasSuffix(name, ".conf") || name == "nginx.conf" {
			out = append(out, p)
		}
	}
	return out
}

func parseNginxIncludes(src string) []string {
	var out []string
	for _, line := range stripNginxComments(src) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "include" {
			continue
		}
		p := strings.TrimSuffix(fields[1], ";")
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hostJoin(root, p string) string {
	if root == "" {
		return p
	}
	return filepath.Join(root, strings.TrimPrefix(p, "/"))
}

func nginxIncludeAllowed(path, hostRoot string) bool {
	full := path
	if hostRoot != "" {
		full = hostJoin(hostRoot, path)
	}
	allow := []string{
		"/etc/nginx",
		"/www/server/nginx",
		"/www/server/panel/vhost/nginx",
	}
	for _, a := range allow {
		prefix := a
		if hostRoot != "" {
			prefix = filepath.Join(hostRoot, a)
		}
		if strings.HasPrefix(full, prefix) {
			return true
		}
	}
	return false
}

func collectNginxIncludes(seen map[string][]byte, hostRoot string) []string {
	var out []string
	for _, data := range seen {
		for _, inc := range parseNginxIncludes(string(data)) {
			if !nginxIncludeAllowed(inc, hostRoot) {
				continue
			}
			pattern := inc
			if hostRoot != "" {
				pattern = hostJoin(hostRoot, inc)
			}
			matches, err := filepath.Glob(pattern)
			if err != nil {
				continue
			}
			for _, m := range matches {
				if strings.HasSuffix(m, ".conf") || filepath.Base(m) == "nginx.conf" {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// ParseNginxUpstreams maps upstream name -> first server address.
func ParseNginxUpstreams(src string) map[string]string {
	out := map[string]string{}
	lines := stripNginxComments(src)
	for i := 0; i < len(lines); i++ {
		f := strings.Fields(lines[i])
		if len(f) < 2 || f[0] != "upstream" {
			continue
		}
		name := strings.TrimSuffix(f[1], "{")
		for j := i; j < len(lines); j++ {
			sf := strings.Fields(lines[j])
			if len(sf) >= 2 && sf[0] == "server" {
				addr := strings.TrimSuffix(sf[1], ";")
				out[name] = addr
				break
			}
			if strings.Contains(lines[j], "}") && j > i {
				break
			}
		}
	}
	return out
}

// ParseNginxProxies extracts server/location/proxy_pass rows.
func ParseNginxProxies(src string, upstreams map[string]string) []hostsnap.Proxy {
	lines := stripNginxComments(src)
	var proxies []hostsnap.Proxy
	var listen, serverName string
	inServer := false
	depth := 0
	serverDepth := 0
	loc := ""
	locDepth := 0
	for _, line := range lines {
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		fields := strings.Fields(line)
		if !inServer && len(fields) > 0 && fields[0] == "server" {
			inServer = true
			serverDepth = depth
			listen, serverName = "", ""
		}
		if inServer && len(fields) >= 2 {
			switch fields[0] {
			case "listen":
				listen = strings.TrimSuffix(fields[1], ";")
			case "server_name":
				serverName = strings.TrimSuffix(fields[1], ";")
			case "location":
				loc = strings.TrimSuffix(fields[1], "{")
				if loc == "" && len(fields) > 2 {
					loc = strings.TrimSuffix(fields[2], "{")
					if fields[1] == "=" || fields[1] == "~" || fields[1] == "~*" || fields[1] == "^~" {
						loc = strings.TrimSuffix(fields[2], "{")
					}
				}
				locDepth = depth
			case "proxy_pass":
				up := strings.TrimSuffix(fields[1], ";")
				up = strings.TrimPrefix(up, "http://")
				up = strings.TrimPrefix(up, "https://")
				up = strings.TrimSuffix(up, "/")
				if mapped, ok := upstreams[up]; ok {
					up = mapped
				}
				proxies = append(proxies, hostsnap.Proxy{
					ServerName: serverName,
					Listen:     listen,
					ListenPort: listenPort(listen),
					Location:   loc,
					Upstream:   up,
				})
			}
		}
		depth += opens - closes
		if loc != "" && depth <= locDepth {
			loc = ""
		}
		if inServer && depth <= serverDepth && closes > 0 {
			inServer = false
			listen, serverName, loc = "", "", ""
		}
	}
	return proxies
}

func stripNginxComments(src string) []string {
	var lines []string
	for _, raw := range strings.Split(src, "\n") {
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func listenPort(listen string) int {
	listen = strings.Trim(listen, "[]")
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		listen = listen[i+1:]
	}
	n, err := strconv.Atoi(listen)
	if err != nil {
		return 0
	}
	return n
}

func dedupeProxies(in []hostsnap.Proxy) []hostsnap.Proxy {
	seen := map[string]struct{}{}
	var out []hostsnap.Proxy
	for _, p := range in {
		key := p.ServerName + "|" + p.Listen + "|" + p.Location + "|" + p.Upstream
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

// EffectiveProxies keeps reverse proxies whose listen port is currently bound.
func EffectiveProxies(proxies []hostsnap.Proxy, ports []hostsnap.Port) []hostsnap.Proxy {
	bound := map[int]bool{}
	for _, p := range ports {
		bound[p.Port] = true
	}
	var out []hostsnap.Proxy
	for _, px := range proxies {
		if px.ListenPort > 0 && !bound[px.ListenPort] {
			continue
		}
		if px.Upstream == "" {
			continue
		}
		out = append(out, px)
	}
	return out
}
