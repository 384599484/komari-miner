//go:build darwin

package probe

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var darwinNetState = struct {
	sync.Mutex
	rx uint64
	tx uint64
	at time.Time
}{}

func readSystem() (System, error) {
	s := System{
		OS:       "macos",
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}
	s.Hostname, _ = os.Hostname()
	if out, err := runCommand("uname", "-r"); err == nil {
		s.Kernel = strings.TrimSpace(out)
	}
	s.CPUName = darwinCPUName()
	s.CPUUsage = darwinCPUUsage()
	readDarwinMemory(&s)
	readDarwinSwap(&s)
	readDarwinLoad(&s)
	readDarwinDisk(&s)
	readDarwinNetwork(&s)
	readDarwinUptime(&s)
	s.TCP = darwinConnectionCount("tcp")
	s.UDP = darwinConnectionCount("udp")
	if out, err := runCommand("ps", "-ax"); err == nil {
		s.Processes = max(0, strings.Count(strings.TrimSpace(out), "\n"))
	}
	return s, nil
}

func darwinCPUName() string {
	if out, err := runCommand("sysctl", "-n", "machdep.cpu.brand_string"); err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out)
	}
	if out, err := runCommand("sysctl", "-n", "hw.model"); err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}

func darwinCPUUsage() float64 {
	out, err := runCommand("top", "-l", "1", "-n", "0", "-s", "0")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "CPU usage:") {
			continue
		}
		i := strings.Index(line, "idle")
		if i < 0 {
			continue
		}
		prefix := line[:i]
		j := strings.LastIndex(prefix, ",")
		if j >= 0 {
			prefix = prefix[j+1:]
		}
		idle := parseFloat(prefix)
		usage := 100 - idle
		if usage < 0 {
			usage = 0
		}
		return round2(usage)
	}
	return 0
}

func readDarwinMemory(s *System) {
	if out, err := runCommand("sysctl", "-n", "hw.memsize"); err == nil {
		s.RAMTotal, _ = strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	}
	out, err := runCommand("vm_stat")
	if err != nil {
		return
	}
	pageSize := int64(4096)
	free, inactive, speculative, purgeable := int64(0), int64(0), int64(0), int64(0)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "page size of") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "of" && i+1 < len(fields) {
					pageSize, _ = strconv.ParseInt(strings.Trim(fields[i+1], ".,"), 10, 64)
				}
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		key := strings.TrimSuffix(strings.Join(fields[:2], " "), ":")
		v, _ := strconv.ParseInt(strings.TrimSuffix(fields[len(fields)-1], "."), 10, 64)
		switch key {
		case "Pages free":
			free = v
		case "Pages inactive":
			inactive = v
		case "Pages speculative":
			speculative = v
		case "Pages purgeable":
			purgeable = v
		}
	}
	available := (free + inactive + speculative + purgeable) * pageSize
	if s.RAMTotal > 0 && available <= s.RAMTotal {
		s.RAMUsed = s.RAMTotal - available
	}
}

func readDarwinSwap(s *System) {
	out, err := runCommand("sysctl", "-n", "vm.swapusage")
	if err != nil {
		return
	}
	// total = 2048.00M  used = 0.00M  free = 2048.00M
	fields := strings.Fields(out)
	for i := 0; i+2 < len(fields); i += 3 {
		key := strings.TrimSuffix(fields[i], "=")
		val := parseDarwinSize(fields[i+2])
		switch key {
		case "total":
			s.SwapTotal = val
		case "used":
			s.SwapUsed = val
		}
	}
}

func parseDarwinSize(v string) int64 {
	v = strings.TrimSpace(v)
	mult := int64(1)
	switch {
	case strings.HasSuffix(v, "G"):
		mult = 1024 * 1024 * 1024
	case strings.HasSuffix(v, "M"):
		mult = 1024 * 1024
	case strings.HasSuffix(v, "K"):
		mult = 1024
	}
	v = strings.TrimRight(v, "GMK")
	f, _ := strconv.ParseFloat(v, 64)
	return int64(f * float64(mult))
}

func readDarwinLoad(s *System) {
	out, err := runCommand("sysctl", "-n", "vm.loadavg")
	if err != nil {
		return
	}
	out = strings.Trim(strings.TrimSpace(out), "{}")
	fields := strings.Fields(out)
	if len(fields) >= 3 {
		s.Load1, _ = strconv.ParseFloat(fields[0], 64)
		s.Load5, _ = strconv.ParseFloat(fields[1], 64)
		s.Load15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

func readDarwinDisk(s *System) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return
	}
	bs := float64(st.Bsize)
	s.DiskTotal = int64(float64(st.Blocks) * bs)
	s.DiskUsed = int64(float64(st.Blocks-st.Bfree) * bs)
}

func readDarwinNetwork(s *System) {
	out, err := runCommand("netstat", "-ibn")
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var rx, tx uint64
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		name := fields[0]
		if name == "lo0" || seen[name] {
			continue
		}
		r, err1 := strconv.ParseUint(fields[6], 10, 64)
		t, err2 := strconv.ParseUint(fields[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		seen[name] = true
		rx += r
		tx += t
	}
	if len(seen) == 0 {
		return
	}
	s.NetTotalDown = int64(rx)
	s.NetTotalUp = int64(tx)
	darwinNetState.Lock()
	defer darwinNetState.Unlock()
	now := time.Now()
	if !darwinNetState.at.IsZero() && rx >= darwinNetState.rx && tx >= darwinNetState.tx {
		secs := now.Sub(darwinNetState.at).Seconds()
		if secs > 0.2 {
			s.NetDown = int64(float64(rx-darwinNetState.rx) / secs)
			s.NetUp = int64(float64(tx-darwinNetState.tx) / secs)
		}
	}
	darwinNetState.rx, darwinNetState.tx, darwinNetState.at = rx, tx, now
}

func readDarwinUptime(s *System) {
	out, err := runCommand("sysctl", "-n", "kern.boottime")
	if err != nil {
		return
	}
	if i := strings.Index(out, "sec ="); i >= 0 {
		rest := out[i+len("sec ="):]
		fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(rest), ","))
		if len(fields) > 0 {
			sec, _ := strconv.ParseInt(strings.Trim(fields[0], ","), 10, 64)
			if sec > 0 {
				s.Uptime = time.Now().Unix() - sec
			}
		}
	}
}

func darwinConnectionCount(proto string) int {
	out, err := runCommand("netstat", "-an", "-p", proto)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "ESTABLISHED") || (proto == "udp" && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "Active") && !strings.HasPrefix(line, "Proto")) {
			n++
		}
	}
	return n
}
