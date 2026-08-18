package event

import "testing"

func TestAllowedTransition(t *testing.T) {
	ok := [][2]string{
		{"queued", "running"},
		{"queued", "cancelled"},
		{"running", "succeeded"},
		{"running", "failed"},
		{"running", "waiting_input"},
		{"waiting_input", "running"},
		{"blocked", "failed"},
	}
	for _, c := range ok {
		if !AllowedTransition(c[0], c[1]) {
			t.Errorf("expected %s->%s allowed", c[0], c[1])
		}
	}
	bad := [][2]string{
		{"succeeded", "running"}, // terminal
		{"failed", "succeeded"},
		{"queued", "succeeded"}, // must go through running
		{"cancelled", "running"},
	}
	for _, c := range bad {
		if AllowedTransition(c[0], c[1]) {
			t.Errorf("expected %s->%s rejected", c[0], c[1])
		}
	}
}

func TestRunSeverity(t *testing.T) {
	cases := map[string]string{
		"failed":    "error",
		"timed_out": "error",
		"blocked":   "warning",
		"running":   "info",
		"succeeded": "info",
	}
	for status, want := range cases {
		if got := RunSeverity(status); got != want {
			t.Errorf("RunSeverity(%s) = %s, want %s", status, got, want)
		}
	}
}

func TestValidators(t *testing.T) {
	if !KnownType(TypeMetricSample) || KnownType("bogus.type") {
		t.Error("KnownType incorrect")
	}
	if !ValidServiceType("agent") || ValidServiceType("nope") {
		t.Error("ValidServiceType incorrect")
	}
	if !ValidRunStatus("succeeded") || ValidRunStatus("weird") {
		t.Error("ValidRunStatus incorrect")
	}
	if !IsTerminal("failed") || IsTerminal("running") {
		t.Error("IsTerminal incorrect")
	}
	if !ValidServiceKey("nginx.service") || ValidServiceKey("user@1000.service") || ValidServiceKey("BAD KEY") {
		t.Error("ValidServiceKey incorrect")
	}
	st, sum, sev := UnitProjection("failed", "failed", "sshd")
	if st != "failed" || sev != "error" || sum == "" {
		t.Errorf("UnitProjection failed: %s %s %s", st, sum, sev)
	}
}
