package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProc(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadMemory(t *testing.T) {
	root := t.TempDir()
	writeProc(t, root, "meminfo", `MemTotal:       16384 kB
MemFree:         1000 kB
MemAvailable:    8192 kB
Buffers:          100 kB
Cached:           200 kB
SwapTotal:       2048 kB
SwapFree:        2000 kB
`)
	c := &Collector{ProcRoot: root, prevNet: map[string][2]uint64{}}
	used, total, swapUsed, swapTotal := c.readMemory()
	if total == nil || *total != 16384*1024 {
		t.Fatalf("total = %v", total)
	}
	if used == nil || *used != (16384-8192)*1024 {
		t.Fatalf("used = %v want %d", used, (16384-8192)*1024)
	}
	if swapTotal == nil || *swapTotal != 2048*1024 {
		t.Fatalf("swapTotal = %v", swapTotal)
	}
	if swapUsed == nil || *swapUsed != (2048-2000)*1024 {
		t.Fatalf("swapUsed = %v", swapUsed)
	}
}

func TestReadCPUPercentDelta(t *testing.T) {
	root := t.TempDir()
	c := &Collector{ProcRoot: root, prevNet: map[string][2]uint64{}}

	// First sample: idle=100 (idle+iowait=90+10), total=200.
	writeProc(t, root, "stat", "cpu  50 0 40 90 10 0 10 0 0 0\n")
	if p := c.readCPUPercent(); p != nil {
		t.Fatalf("first sample should be nil, got %v", *p)
	}
	// Second sample: idle grows by 50 (idle 90->130, iowait 10->20 => +50),
	// total grows by 100 => busy fraction = 1 - 50/100 = 50%.
	writeProc(t, root, "stat", "cpu  80 0 60 130 20 0 10 0 0 0\n")
	p := c.readCPUPercent()
	if p == nil {
		t.Fatal("second sample should return a value")
	}
	if *p < 49.9 || *p > 50.1 {
		t.Fatalf("cpu percent = %v, want ~50", *p)
	}
}

func TestReadNetworkRate(t *testing.T) {
	root := t.TempDir()
	c := &Collector{ProcRoot: root, prevNet: map[string][2]uint64{}}
	header := "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets\n"
	writeProc(t, root, "net/dev", header+"  eth0: 1000 0 0 0 0 0 0 0 500 0 0 0 0 0 0 0\n  lo: 5 0 0 0 0 0 0 0 5 0 0 0 0 0 0 0\n")
	// First call establishes baseline (elapsed ignored).
	if rx, _, _ := c.readNetwork(1.0, nil); rx != nil {
		t.Fatalf("first network sample should be nil, got %v", *rx)
	}
	// Second call: +2000 rx over 2s => 1000 bytes/s; lo excluded.
	writeProc(t, root, "net/dev", header+"  eth0: 3000 0 0 0 0 0 0 0 1500 0 0 0 0 0 0 0\n  lo: 9 0 0 0 0 0 0 0 9 0 0 0 0 0 0 0\n")
	rx, tx, ifaces := c.readNetwork(2.0, nil)
	if rx == nil || *rx != 1000 {
		t.Fatalf("rx = %v, want 1000", rx)
	}
	if tx == nil || *tx != 500 {
		t.Fatalf("tx = %v, want 500", tx)
	}
	if _, ok := ifaces["lo"]; ok {
		t.Fatal("lo should be excluded")
	}
	if _, ok := ifaces["eth0"]; !ok {
		t.Fatal("eth0 should be present")
	}
}
