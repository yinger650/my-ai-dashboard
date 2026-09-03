package collector

import "testing"

const showSample = `Id=nginx.service
LoadState=loaded
ActiveState=active
SubState=running
Description=A high performance web server
MainPID=441
ExecStart={ path=/usr/sbin/nginx ; argv[]=/usr/sbin/nginx -g daemon on; master_process on; ; ignore_errors=no }
FragmentPath=/usr/lib/systemd/system/nginx.service

Id=user@1000.service
LoadState=loaded
ActiveState=active
SubState=running
Description=User Manager for UID 1000
MainPID=900
ExecStart={ path=/usr/lib/systemd/systemd ; argv[]=/usr/lib/systemd/systemd --user }

Id=sshd.service
LoadState=loaded
ActiveState=failed
SubState=failed
Description=OpenSSH server daemon
MainPID=0
ExecStart={ path=/usr/sbin/sshd ; argv[]=/usr/sbin/sshd -D $OPTIONS $CRYPTO_POLICY }
FragmentPath=/usr/lib/systemd/system/sshd.service

Id=systemd-journald.service
LoadState=loaded
ActiveState=active
SubState=running
Description=Journal Service
`

func TestParseAndFilterSystemctlShow(t *testing.T) {
	units := ParseSystemctlShow(showSample)
	if len(units) != 4 {
		t.Fatalf("parsed %d units: %+v", len(units), units)
	}
	if units[0].Unit != "nginx.service" || units[0].Active != "active" {
		t.Fatalf("nginx: %+v", units[0])
	}
	if units[0].MainPID != 441 {
		t.Fatalf("nginx MainPID = %d", units[0].MainPID)
	}
	if units[0].Path == "" {
		t.Fatalf("nginx path empty: %+v", units[0])
	}
	if ProcExe(441) == "" && units[0].Path != "/usr/sbin/nginx" {
		t.Fatalf("nginx ExecStart path = %q", units[0].Path)
	}
	if units[2].Path != "/usr/sbin/sshd" {
		t.Fatalf("sshd path = %q", units[2].Path)
	}

	filtered := FilterUnits(units, false, []string{"nginx.service", "sshd.service"}, nil)
	if len(filtered) != 2 {
		t.Fatalf("include filter = %d", len(filtered))
	}

	all := FilterUnits(units, true, nil, []string{"systemd-", "user@"})
	if len(all) != 2 {
		t.Fatalf("include_all = %d want 2 (nginx+sshd), got %+v", len(all), all)
	}
	snap := UnitsToSnapshot(filtered)
	if len(snap.Units) != 2 || snap.Units[0].Name == "" {
		t.Fatalf("snapshot: %+v", snap)
	}
	if snap.Units[1].Path != "/usr/sbin/sshd" {
		t.Fatalf("snapshot sshd path = %q", snap.Units[1].Path)
	}
}
