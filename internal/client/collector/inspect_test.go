package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestIsCronNoise(t *testing.T) {
	if !IsCronNoise("flock -xn /tmp/stargate.lock /usr/local/qcloud/stargate/admin/start.sh") {
		t.Fatal("qcloud stargate should be noise")
	}
	if !IsCronNoise("/usr/local/qcloud/tat_agent/tat_agent") {
		t.Fatal("tat_agent should be noise")
	}
	if IsCronNoise("/usr/bin/backup") {
		t.Fatal("normal job should not be noise")
	}
}

func TestReadCronJobsHidesQcloud(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "*/1 * * * * root flock -xn /tmp/lock /usr/local/qcloud/stargate/admin/start.sh\n0 3 * * * root /usr/bin/backup\n"
	if err := os.WriteFile(filepath.Join(root, "etc", "crontab"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := ReadCronJobs(root, nil)
	if len(jobs) != 1 || jobs[0].Command != "/usr/bin/backup" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestReadCronExecutionsDoesNotReplayHistory(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "var", "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	p := filepath.Join(logDir, "cron")
	var body strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, "%s box CRON[%d]: (root) CMD (/usr/bin/old-%d)\n", old, i, i)
	}
	fmt.Fprintf(&body, "%s box CRON[99]: (root) CMD (flock -xn /tmp/x /usr/local/qcloud/stargate/admin/start.sh)\n", recent)
	fmt.Fprintf(&body, "%s box CRON[100]: (root) CMD (/usr/bin/backup)\n", recent)
	if err := os.WriteFile(p, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	tail := &CronTail{}
	first := ReadCronExecutions(root, []string{"/var/log/cron"}, nil, tail)
	if len(first) != 1 || !strings.Contains(first[0].Command, "backup") {
		t.Fatalf("first = %+v", first)
	}
	if !tail.Primed {
		t.Fatal("expected primed after first read")
	}

	second := ReadCronExecutions(root, []string{"/var/log/cron"}, nil, tail)
	if len(second) != 0 {
		t.Fatalf("second replay = %+v", second)
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	if _, err := fmt.Fprintf(f, "%s box CRON[101]: (root) CMD (/usr/bin/tick)\n", newer); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	third := ReadCronExecutions(root, []string{"/var/log/cron"}, nil, tail)
	if len(third) != 1 || !strings.Contains(third[0].Command, "tick") {
		t.Fatalf("incremental = %+v", third)
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
