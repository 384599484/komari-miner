//go:build darwin

package probe

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var psLineRe = regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+([0-9.]+)\s+(\d+)\s+(\S+)\s+(.*)$`)

func scanMiners(gpus []GPU, extraPatterns []string) ([]Miner, error) {
	out, err := runCommand("ps", "-axo", "pid=,user=,pcpu=,rss=,etime=,command=")
	if err != nil {
		return nil, err
	}
	miners := make([]Miner, 0, 4)
	for _, line := range strings.Split(out, "\n") {
		m := psLineRe.FindStringSubmatch(line)
		if len(m) != 7 {
			continue
		}
		pid, _ := strconv.Atoi(m[1])
		if pid <= 1 {
			continue
		}
		cpu, _ := strconv.ParseFloat(m[3], 64)
		rssKB, _ := strconv.ParseInt(m[4], 10, 64)
		uptime := parseElapsedSeconds(m[5])
		command := strings.TrimSpace(m[6])
		args := strings.Fields(command)
		if len(args) == 0 {
			continue
		}
		if !matchMinerName(args[0], extraPatterns) && !looksLikeMinerArgs(args) {
			continue
		}
		candidate := processCandidate{
			PID:       pid,
			Name:      strings.TrimSpace(m[2]),
			Exe:       args[0],
			User:      strings.TrimSpace(m[2]),
			Command:   command,
			Args:      args,
			CPU:       round2(cpu),
			Memory:    rssKB * 1024,
			Uptime:    uptime,
			StartTime: time.Now().Add(-time.Duration(uptime) * time.Second),
		}
		if miner, ok := buildMiner(candidate, gpus, extraPatterns); ok {
			miners = append(miners, miner)
		}
	}
	return miners, nil
}

func parseElapsedSeconds(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	days := int64(0)
	if i := strings.Index(v, "-"); i >= 0 {
		days, _ = strconv.ParseInt(v[:i], 10, 64)
		v = v[i+1:]
	}
	parts := strings.Split(v, ":")
	var total int64
	for _, p := range parts {
		n, _ := strconv.ParseInt(p, 10, 64)
		total = total*60 + n
	}
	return days*86400 + total
}
