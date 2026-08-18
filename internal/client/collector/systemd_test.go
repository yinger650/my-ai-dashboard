package collector

import "testing"

const showSample = `Id=nginx.service
LoadState=loaded
ActiveState=active
SubState=running
Description=A high performance web server

Id=user@1000.service
LoadState=loaded
ActiveState=active
SubState=running
Description=User Manager for UID 1000

Id=sshd.service
LoadState=loaded
ActiveState=failed
SubState=failed
Description=OpenSSH server daemon

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
}
