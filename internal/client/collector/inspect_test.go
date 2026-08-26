package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentboard/internal/client/hostsnap"
)

func TestParseSS(t *testing.T) {
	in := `tcp   LISTEN 0      511          0.0.0.0:80        0.0.0.0:*    users:(("nginx",pid=91,fd=6))
tcp   LISTEN 0      128          0.0.0.0:22        0.0.0.0:*    users:(("sshd",pid=80,fd=3))
udp   UNCONN 0      0         127.0.0.1:323       0.0.0.0:*
`
	ports := ParseSS(in)
	if len(ports) != 3 {
		t.Fatalf("ports = %d %+v", len(ports), ports)
	}
	if ports[0].Process != "nginx" || ports[0].Port != 80 || ports[0].PID != 91 {
		t.Fatalf("nginx: %+v", ports[0])
	}
}

func TestParseDockerPS(t *testing.T) {
	in := "abc\tweb\tnginx:latest\tRunning\t0.0.0.0:80->80/tcp\nxyz\told\talpine\texited\t\n"
	cs := ParseDockerPS(in)
	if len(cs) != 2 || cs[0].State != "running" || cs[1].State != "exited" {
		t.Fatalf("%+v", cs)
	}
}

func TestParseCrontabHidesComments(t *testing.T) {
	src := `# disabled
SHELL=/bin/sh
0 3 * * * root /usr/bin/backup
*/5 * * * * root /usr/bin/tick
@daily root /usr/bin/daily
`
	jobs := ParseCrontab(src, true)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d %+v", len(jobs), jobs)
	}
	if jobs[0].Schedule != "0 3 * * *" || jobs[0].User != "root" {
		t.Fatalf("first: %+v", jobs[0])
	}
}

func TestParseCronLogDedup(t *testing.T) {
	seen := map[string]bool{}
	src := `2026-08-26T03:20:01+08:00 box CRON[12]: (root) CMD (/usr/bin/backup)
2026-08-26T03:20:01+08:00 box CRON[12]: (root) CMD (/usr/bin/backup)
`
	ex := ParseCronLog(src, seen)
	if len(ex) != 1 {
		t.Fatalf("execs = %d", len(ex))
	}
	if ex[0].User != "root" || !strings.Contains(ex[0].Command, "backup") {
		t.Fatalf("%+v", ex[0])
	}
	if n := len(ParseCronLog(src, seen)); n != 0 {
		t.Fatalf("second pass = %d", n)
	}
}

func TestParseNginxAndEffective(t *testing.T) {
	src := `
# comment
upstream board {
    server 127.0.0.1:8090;
}
server {
    listen 80;
    server_name skip.example;
}
server {
    listen 443 ssl;
    server_name board.yinger650.com;
    location / {
        proxy_pass http://board;
    }
}
server {
    listen 8081;
    server_name hidden.example;
    location / {
        proxy_pass http://127.0.0.1:9000;
    }
}
`
	ups := ParseNginxUpstreams(src)
	px := ParseNginxProxies(src, ups)
	if len(px) != 2 {
		t.Fatalf("proxies = %d %+v", len(px), px)
	}
	eff := EffectiveProxies(px, []hostsnap.Port{{Port: 443, Process: "nginx"}})
	if len(eff) != 1 || eff[0].ServerName != "board.yinger650.com" || eff[0].Upstream != "127.0.0.1:8090" {
		t.Fatalf("effective = %+v", eff)
	}
}

func TestReadCronJobsFromRoot(t *testing.T) {
	root := t.TempDir()
	etc := filepath.Join(root, "etc")
	if err := os.MkdirAll(filepath.Join(etc, "cron.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(etc, "crontab"), []byte("0 1 * * * root /bin/one\n# off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(etc, "cron.d", "jobs"), []byte("0 2 * * * root /bin/two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := ReadCronJobs(root, nil)
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d %+v", len(jobs), jobs)
	}
}

func TestReadDockerUnavailable(t *testing.T) {
	info := ReadDocker(func(name string, args ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	})
	if info.Available {
		t.Fatal("expected unavailable")
	}
}

func TestReadDockerOK(t *testing.T) {
	info := ReadDocker(func(name string, args ...string) ([]byte, error) {
		if args[0] == "ps" {
			return []byte("id1\tweb\tnginx\trunning\t80/tcp\n"), nil
		}
		return []byte("sha1\nsha2\n"), nil
	})
	if !info.Available || info.ImageCount != 2 || len(info.Containers) != 1 {
		t.Fatalf("%+v", info)
	}
}
