package collector

import (
	"os"
	"strings"
	"testing"

	"agentboard/internal/client/hostsnap"
)

func TestParseExecStartPath(t *testing.T) {
	got := ParseExecStartPath("{ path=/usr/sbin/sshd ; argv[]=/usr/sbin/sshd -D $OPTIONS ; ignore_errors=no }")
	if got != "/usr/sbin/sshd" {
		t.Fatalf("brace form = %q", got)
	}
	if ParseExecStartPath("/usr/sbin/nginx -g 'daemon on;'") != "/usr/sbin/nginx" {
		t.Fatal("simple form")
	}
	if ParseExecStartPath("") != "" {
		t.Fatal("empty")
	}
}

func TestProcExeSelf(t *testing.T) {
	p := ProcExe(os.Getpid())
	if p == "" {
		t.Fatal("expected current process exe")
	}
	if !strings.Contains(p, "/") {
		t.Fatalf("exe = %q", p)
	}
}

func TestUnitPathAndPortExe(t *testing.T) {
	units := []hostsnap.Unit{{Unit: "nginx.service", Path: "/usr/sbin/nginx"}}
	if unitPath(units, "nginx") != "/usr/sbin/nginx" {
		t.Fatal("unitPath")
	}
	ports := []hostsnap.Port{{Process: "dockerd", Exe: "/usr/bin/dockerd"}}
	if firstPortExe(ports, "dockerd") != "/usr/bin/dockerd" {
		t.Fatal("firstPortExe")
	}
}
