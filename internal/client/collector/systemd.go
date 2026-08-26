package collector

import (
	"strings"

	"agentboard/internal/event"
)

// Unit is a systemd unit snapshot.
type Unit struct {
	Unit        string
	Load        string
	Active      string
	Sub         string
	Description string
}

// ReadSystemdUnits runs systemctl show and returns matching units.
func ReadSystemdUnits(includeAll bool, include []string) ([]Unit, error) {
	return ReadSystemdUnitsCmd(DefaultCommander, includeAll, include)
}

// ReadSystemdUnitsCmd is ReadSystemdUnits with an injectable commander.
func ReadSystemdUnitsCmd(run Commander, includeAll bool, include []string) ([]Unit, error) {
	if run == nil {
		run = DefaultCommander
	}
	args := []string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,SubState,Description"}
	if includeAll {
		args = append(args, "*.service")
	} else {
		if len(include) == 0 {
			return nil, nil
		}
		args = append(args, include...)
	}
	out, err := run("systemctl", args...)
	if err != nil {
		return nil, err
	}
	return ParseSystemctlShow(string(out)), nil
}

// ParseSystemctlShow parses `systemctl show --property=...` output.
func ParseSystemctlShow(output string) []Unit {
	var units []Unit
	var cur Unit
	flush := func() {
		if cur.Unit != "" {
			units = append(units, cur)
		}
		cur = Unit{}
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "Id":
			cur.Unit = strings.TrimSpace(v)
		case "LoadState":
			cur.Load = v
		case "ActiveState":
			cur.Active = v
		case "SubState":
			cur.Sub = v
		case "Description":
			cur.Description = v
		}
	}
	flush()
	return units
}

// FilterUnits keeps include_all (minus prefixes) or an explicit include list.
func FilterUnits(units []Unit, includeAll bool, include, excludePrefixes []string) []Unit {
	want := map[string]bool{}
	for _, n := range include {
		want[n] = true
	}
	var out []Unit
	for _, u := range units {
		if u.Unit == "" || strings.Contains(u.Unit, "@") {
			continue
		}
		if !event.ValidServiceKey(u.Unit) {
			continue
		}
		if includeAll {
			if hasAnyPrefix(u.Unit, excludePrefixes) {
				continue
			}
			out = append(out, u)
			continue
		}
		if want[u.Unit] {
			out = append(out, u)
		}
	}
	return out
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// UnitsToSnapshot converts filtered units to a service snapshot payload.
func UnitsToSnapshot(units []Unit) event.ServiceSnapshot {
	out := make([]event.SnapshotUnit, 0, len(units))
	for _, u := range units {
		name := u.Description
		if name == "" {
			name = strings.TrimSuffix(u.Unit, ".service")
		}
		out = append(out, event.SnapshotUnit{
			Unit:        u.Unit,
			Load:        u.Load,
			Active:      u.Active,
			Sub:         u.Sub,
			Description: u.Description,
			Name:        name,
		})
	}
	return event.ServiceSnapshot{Units: out}
}
