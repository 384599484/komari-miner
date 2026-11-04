//go:build linux

package probe

import (
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"
)

func scanMiners(gpus []GPU, extraPatterns []string) ([]Miner, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	uptime := readUptimeSeconds()
	boot := linuxBootTime()
	userCache := map[int]string{}
	miners := make([]Miner, 0, 4)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		base := "/proc/" + entry.Name()
		comm := strings.TrimSpace(readSmallFile(base + "/comm"))
		cmdlineRaw := readSmallFile(base + "/cmdline")
		if strings.TrimSpace(cmdlineRaw) == "" {
			continue // kernel thread or exiting process
		}
		args := splitNUL(cmdlineRaw)
		if len(args) == 0 {
			continue
		}
		exe, _ := os.Readlink(base + "/exe")
		if exe == "" {
			exe = args[0]
		}
		if !matchMinerName(comm, extraPatterns) && !matchMinerName(exe, extraPatterns) && !looksLikeMinerArgs(args) {
			continue
		}

		utime, stime, startTicks := readProcTimes(base + "/stat")
		uid := readProcUID(base + "/status")
		mem := readProcRSS(base + "/statm")
		processUptime := int64(0)
		if uptime > 0 && startTicks > 0 && uptime > startTicks/100 {
			processUptime = int64(uptime) - int64(startTicks/100)
		}
		startTime := time.Time{}
		if !boot.IsZero() && startTicks > 0 {
			startTime = boot.Add(time.Duration(startTicks/100) * time.Second)
		}
		candidate := processCandidate{
			PID:       pid,
			Name:      comm,
			Exe:       exe,
			User:      linuxUsername(uid, userCache),
			Command:   strings.Join(args, " "),
			Args:      args,
			CPU:       linuxProcCPU(utime, stime, startTicks, uptime),
			Memory:    mem,
			Uptime:    processUptime,
			StartTime: startTime,
		}
		if m, ok := buildMiner(candidate, gpus, extraPatterns); ok {
			miners = append(miners, m)
		}
	}
	return miners, nil
}

func splitNUL(raw string) []string {
	parts := strings.Split(raw, "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func readProcTimes(path string) (utime, stime, startTicks uint64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, 0
	}
	// Field 2 (comm) may contain spaces and parentheses. Everything after the
	// final ')' is space-separated from field 3 onwards.
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return 0, 0, 0
	}
	fields := strings.Fields(text[end+2:])
	// fields[0] is field 3 (state); utime is field 14, stime 15, starttime 22.
	if len(fields) < 20 {
		return 0, 0, 0
	}
	utime, _ = strconv.ParseUint(fields[11], 10, 64)
	stime, _ = strconv.ParseUint(fields[12], 10, 64)
	startTicks, _ = strconv.ParseUint(fields[19], 10, 64)
	return
}

func readProcUID(path string) int {
	data := readSmallFile(path)
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				uid, _ := strconv.Atoi(fields[1])
				return uid
			}
		}
	}
	return -1
}

func readProcRSS(path string) int64 {
	fields := strings.Fields(readSmallFile(path))
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseInt(fields[1], 10, 64)
	const pageSize = 4096
	return pages * pageSize
}

func linuxUsername(uid int, cache map[int]string) string {
	if uid < 0 {
		return ""
	}
	if name, ok := cache[uid]; ok {
		return name
	}
	name := strconv.Itoa(uid)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	cache[uid] = name
	return name
}
