//go:build linux

package probe

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var linuxCPUState = struct {
	sync.Mutex
	total uint64
	idle  uint64
}{}

var linuxNetState = struct {
	sync.Mutex
	rx uint64
	tx uint64
	at time.Time
}{}

func readSystem() (System, error) {
	s := System{
		OS:       "linux",
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}
	s.Hostname, _ = os.Hostname()
	s.Kernel = strings.TrimSpace(readSmallFile("/proc/sys/kernel/osrelease"))
	s.CPUName = linuxCPUName()
	s.CPUUsage = linuxCPUUsage()
	readLinuxMemory(&s)
	readLinuxLoad(&s)
	readLinuxDisk(&s)
	readLinuxNetwork(&s)
	readLinuxUptime(&s)
	s.TCP = countProcSockets("/proc/net/tcp", "/proc/net/tcp6")
	s.UDP = countProcSockets("/proc/net/udp", "/proc/net/udp6")
	s.Processes = countLinuxProcesses()
	return s, nil
}

func readSmallFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func linuxCPUName() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, ":"); i > 0 {
			key := strings.TrimSpace(line[:i])
			if key == "model name" || key == "Hardware" || key == "cpu model" {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

func linuxCPUUsage() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0
	}
	var total, idle uint64
	for i, f := range fields[1:] {
		v, _ := strconv.ParseUint(f, 10, 64)
		total += v
		if i == 3 || i == 4 { // idle + iowait
			idle += v
		}
	}
	linuxCPUState.Lock()
	defer linuxCPUState.Unlock()
	prevTotal, prevIdle := linuxCPUState.total, linuxCPUState.idle
	linuxCPUState.total, linuxCPUState.idle = total, idle
	if prevTotal == 0 || total <= prevTotal {
		return 0
	}
	dt := float64(total - prevTotal)
	di := float64(idle - prevIdle)
	usage := (dt - di) / dt * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return round2(usage)
}

func readLinuxMemory(s *System) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	vals := map[string]int64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(fields[1], 10, 64)
		vals[strings.TrimSuffix(fields[0], ":")] = v * 1024
	}
	s.RAMTotal = vals["MemTotal"]
	avail := vals["MemAvailable"]
	if avail == 0 {
		avail = vals["MemFree"] + vals["Buffers"] + vals["Cached"]
	}
	s.RAMUsed = s.RAMTotal - avail
	s.SwapTotal = vals["SwapTotal"]
	s.SwapUsed = vals["SwapTotal"] - vals["SwapFree"]
}

func readLinuxLoad(s *System) {
	data := strings.TrimSpace(readSmallFile("/proc/loadavg"))
	fields := strings.Fields(data)
	if len(fields) >= 3 {
		s.Load1, _ = strconv.ParseFloat(fields[0], 64)
		s.Load5, _ = strconv.ParseFloat(fields[1], 64)
		s.Load15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

func readLinuxDisk(s *System) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return
	}
	bs := float64(st.Bsize)
	s.DiskTotal = int64(float64(st.Blocks) * bs)
	s.DiskUsed = int64(float64(st.Blocks-st.Bfree) * bs)
}

func readLinuxNetwork(s *System) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()
	var rx, tx uint64
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if lineNo <= 2 {
			continue
		}
		line := sc.Text()
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if iface == "lo" || strings.HasPrefix(iface, "lo:") {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	s.NetTotalDown = int64(rx)
	s.NetTotalUp = int64(tx)
	linuxNetState.Lock()
	defer linuxNetState.Unlock()
	now := time.Now()
	if !linuxNetState.at.IsZero() && rx >= linuxNetState.rx && tx >= linuxNetState.tx {
		secs := now.Sub(linuxNetState.at).Seconds()
		if secs > 0.2 {
			s.NetDown = int64(float64(rx-linuxNetState.rx) / secs)
			s.NetUp = int64(float64(tx-linuxNetState.tx) / secs)
		}
	}
	linuxNetState.rx, linuxNetState.tx, linuxNetState.at = rx, tx, now
}

func readLinuxUptime(s *System) {
	fields := strings.Fields(readSmallFile("/proc/uptime"))
	if len(fields) > 0 {
		f, _ := strconv.ParseFloat(fields[0], 64)
		s.Uptime = int64(f)
	}
}

func countProcSockets(paths ...string) int {
	total := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		n := strings.Count(string(data), "\n")
		if n > 0 {
			total += n - 1 // header
		}
	}
	return total
}

func countLinuxProcesses() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err == nil {
			n++
		}
	}
	return n
}

// linuxProcCPU returns the lifetime average CPU usage of a process. It avoids
// keeping per-PID history while still being useful on a long-running miner.
func linuxProcCPU(utime, stime, startTicks, uptimeSeconds uint64) float64 {
	const clkTck = 100.0
	procSeconds := float64(utime+stime) / clkTck
	started := float64(startTicks) / clkTck
	elapsed := float64(uptimeSeconds) - started
	if elapsed <= 0.5 {
		return 0
	}
	cpu := procSeconds / elapsed * 100
	if cpu < 0 {
		cpu = 0
	}
	if cpu > 100*float64(runtime.NumCPU()) {
		cpu = 100 * float64(runtime.NumCPU())
	}
	return round2(cpu)
}

func readUptimeSeconds() uint64 {
	fields := strings.Fields(readSmallFile("/proc/uptime"))
	if len(fields) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fields[0], 64)
	if f < 0 {
		return 0
	}
	return uint64(f)
}

func linuxBootTime() time.Time {
	data := readSmallFile("/proc/stat")
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "btime ") {
			v, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			return time.Unix(v, 0)
		}
	}
	return time.Time{}
}

func linuxProcExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}
