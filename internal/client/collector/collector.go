// Package collector reads system metrics from /proc and related sources.
package collector

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"agentboard/internal/event"
)

// Collector holds procfs location and previous counters for rate computation.
type Collector struct {
	ProcRoot string

	mu          sync.Mutex
	havePrevCPU bool
	prevCPUIdle uint64
	prevCPUTot  uint64
	prevDiskR   uint64
	prevDiskW   uint64
	prevNet     map[string][2]uint64
	lastTime    time.Time
}

// New returns a Collector rooted at /proc.
func New() *Collector { return &Collector{ProcRoot: "/proc", prevNet: map[string][2]uint64{}} }

func (c *Collector) path(parts ...string) string {
	return filepath.Join(append([]string{c.ProcRoot}, parts...)...)
}

// ReadCPUPercent returns overall CPU busy percent using two /proc/stat samples.
// The first call returns nil (no delta yet).
func (c *Collector) readCPUPercent() *float64 {
	f, err := os.Open(c.path("stat"))
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var idle, total uint64
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		for i, fv := range fields {
			v, _ := strconv.ParseUint(fv, 10, 64)
			total += v
			if i == 3 || i == 4 { // idle + iowait
				idle += v
			}
		}
		break
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.havePrevCPU {
		c.prevCPUIdle, c.prevCPUTot, c.havePrevCPU = idle, total, true
		return nil
	}
	dIdle := float64(idle - c.prevCPUIdle)
	dTotal := float64(total - c.prevCPUTot)
	c.prevCPUIdle, c.prevCPUTot = idle, total
	if dTotal <= 0 {
		return nil
	}
	p := (1.0 - dIdle/dTotal) * 100.0
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return &p
}

func (c *Collector) readLoadAvg() (l1, l5, l15 *float64) {
	data, err := os.ReadFile(c.path("loadavg"))
	if err != nil {
		return nil, nil, nil
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil, nil, nil
	}
	return parseF(fields[0]), parseF(fields[1]), parseF(fields[2])
}

func (c *Collector) readMemory() (used, total, swapUsed, swapTotal *int64) {
	f, err := os.Open(c.path("meminfo"))
	if err != nil {
		return
	}
	defer f.Close()
	vals := map[string]int64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		kb, _ := strconv.ParseInt(parts[1], 10, 64)
		vals[key] = kb * 1024
	}
	if mt, ok := vals["MemTotal"]; ok {
		total = i64p(mt)
		avail, hasAvail := vals["MemAvailable"]
		if !hasAvail {
			avail = vals["MemFree"] + vals["Buffers"] + vals["Cached"]
		}
		used = i64p(mt - avail)
	}
	if st, ok := vals["SwapTotal"]; ok {
		swapTotal = i64p(st)
		swapUsed = i64p(st - vals["SwapFree"])
	}
	return
}

// readDiskIO returns read/write bytes-per-second aggregated across physical
// devices, using the delta since the previous call.
func (c *Collector) readDiskIO(elapsed float64) (rbps, wbps *float64) {
	f, err := os.Open(c.path("diskstats"))
	if err != nil {
		return
	}
	defer f.Close()
	var rSectors, wSectors uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if isVirtualDisk(name) {
			continue
		}
		r, _ := strconv.ParseUint(fields[5], 10, 64)
		wv, _ := strconv.ParseUint(fields[9], 10, 64)
		rSectors += r
		wSectors += wv
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prevDiskR == 0 && c.prevDiskW == 0 && !c.havePrevCPU {
		// first sample handled by havePrevCPU flag elsewhere; still set prev
	}
	if elapsed <= 0 || (c.prevDiskR == 0 && c.prevDiskW == 0) {
		c.prevDiskR, c.prevDiskW = rSectors, wSectors
		return nil, nil
	}
	const sectorSize = 512.0
	rv := float64(rSectors-c.prevDiskR) * sectorSize / elapsed
	wv := float64(wSectors-c.prevDiskW) * sectorSize / elapsed
	c.prevDiskR, c.prevDiskW = rSectors, wSectors
	if rv < 0 {
		rv = 0
	}
	if wv < 0 {
		wv = 0
	}
	return &rv, &wv
}

func (c *Collector) readNetwork(elapsed float64, exclude []string) (rxbps, txbps *float64, ifaces map[string]map[string]float64) {
	f, err := os.Open(c.path("net", "dev"))
	if err != nil {
		return
	}
	defer f.Close()
	ifaces = map[string]map[string]float64{}
	var totalRx, totalTx uint64
	cur := map[string][2]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		if name == "lo" || matchesAny(name, exclude) {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		cur[name] = [2]uint64{rx, tx}
		totalRx += rx
		totalTx += tx
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elapsed <= 0 || len(c.prevNet) == 0 {
		c.prevNet = cur
		return nil, nil, ifaces
	}
	var sumRx, sumTx float64
	for name, v := range cur {
		prev, ok := c.prevNet[name]
		if !ok || v[0] < prev[0] || v[1] < prev[1] {
			continue // counter reset
		}
		rxr := float64(v[0]-prev[0]) / elapsed
		txr := float64(v[1]-prev[1]) / elapsed
		ifaces[name] = map[string]float64{"rx_bps": rxr, "tx_bps": txr}
		sumRx += rxr
		sumTx += txr
	}
	c.prevNet = cur
	return &sumRx, &sumTx, ifaces
}

// Sample builds a metric.sample payload, updating internal rate state.
func (c *Collector) Sample(cfgFilesystems bool, includeMounts, excludeFS, excludeIfaces []string, cpuEnabled, memEnabled, diskEnabled, netEnabled bool) event.MetricSample {
	now := time.Now()
	c.mu.Lock()
	elapsed := 0.0
	if !c.lastTime.IsZero() {
		elapsed = now.Sub(c.lastTime).Seconds()
	}
	c.lastTime = now
	c.mu.Unlock()

	var ms event.MetricSample
	if cpuEnabled {
		ms.CPUPercent = c.readCPUPercent()
		ms.Load1, ms.Load5, ms.Load15 = c.readLoadAvg()
	}
	if memEnabled {
		ms.MemoryUsedBytes, ms.MemoryTotalBytes, ms.SwapUsedBytes, ms.SwapTotalBytes = c.readMemory()
	}
	if diskEnabled {
		ms.DiskReadBps, ms.DiskWriteBps = c.readDiskIO(elapsed)
	}
	if netEnabled {
		ms.NetworkRxBps, ms.NetworkTxBps, ms.Interfaces = c.readNetwork(elapsed, excludeIfaces)
	}
	if cfgFilesystems {
		fss, rootUsed, rootTotal := readFilesystems(includeMounts, excludeFS)
		ms.Filesystems = fss
		ms.RootDiskUsedBytes = rootUsed
		ms.RootDiskTotalBytes = rootTotal
	}
	return ms
}

// Uptime returns system uptime in seconds.
func (c *Collector) Uptime() int64 {
	data, err := os.ReadFile(c.path("uptime"))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fields[0], 64)
	return int64(f)
}

func isVirtualDisk(name string) bool {
	for _, p := range []string{"loop", "ram", "dm-", "sr", "fd"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	// Skip partitions (e.g. sda1) — keep only whole devices for the top-level total.
	if len(name) > 0 {
		last := name[len(name)-1]
		if last >= '0' && last <= '9' && (strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "hd")) {
			return true
		}
	}
	return false
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSuffix(p, "*")
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func parseF(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func i64p(v int64) *int64 { return &v }
