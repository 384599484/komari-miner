package probe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// defaultMinerNames are binary-name fragments of common crypto miners. The
// match is case-insensitive and applies to the executable basename only, so a
// pool URL containing "xmrig" does not create a false positive.
var defaultMinerNames = []string{
	"xmrig", "xmr-stak", "t-rex", "trex", "phoenixminer", "phoenix",
	"gminer", "lolminer", "nbminer", "bzminer", "srbminer", "teamredminer",
	"ethminer", "excavator", "nanominer", "cgminer", "bfgminer", "wildrig",
	"rigel", "miniz", "damominer", "kawpowminer", "prospector", "cpuminer",
	"ttminer", "xminer", "finminer", "claymore", "ethminer-cuda",
	"onezerominer", "srbminer-multi", "lolMiner",
}

type processCandidate struct {
	PID       int
	Name      string
	Exe       string
	User      string
	Command   string
	Args      []string
	CPU       float64
	Memory    int64
	Uptime    int64
	StartTime time.Time
}

// buildMiner decides whether a process is a miner and enriches it with
// command-line metadata and GPU associations.
func buildMiner(p processCandidate, gpus []GPU, extraPatterns []string) (Miner, bool) {
	name := minerDisplayName(p)
	matched := matchMinerName(p.Name, extraPatterns) || matchMinerName(p.Exe, extraPatterns) || matchMinerName(name, extraPatterns)
	if !matched && !looksLikeMinerArgs(p.Args) {
		return Miner{}, false
	}
	m := Miner{
		PID:         p.PID,
		Name:        name,
		Exe:         p.Exe,
		User:        p.User,
		Command:     p.Command,
		CPUPercent:  p.CPU,
		MemoryBytes: p.Memory,
		Uptime:      p.Uptime,
		StartTime:   p.StartTime,
		Status:      "running",
	}
	parseMinerArgs(&m, p.Args)
	enrichFromXMRigConfig(&m, p.Args)
	m.GPUs = gpuIndexesForPID(gpus, p.PID)
	if len(m.GPUs) == 0 && len(gpus) == 1 && looksLikeMinerArgs(p.Args) {
		// A single-GPU rig with no compute-app row is still unambiguous.
		m.GPUs = []int{gpus[0].Index}
	}
	return m, true
}

func minerDisplayName(p processCandidate) string {
	for _, candidate := range []string{p.Exe, p.Name} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		base := filepath.Base(strings.ReplaceAll(candidate, "\\", "/"))
		base = strings.TrimSuffix(base, ".exe")
		base = strings.TrimSuffix(base, ".EXE")
		if base != "" && base != "." {
			return base
		}
	}
	return "miner"
}

func matchMinerName(value string, extra []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(value, "\\", "/")))
	base = strings.TrimSuffix(base, ".exe")
	patterns := append(append([]string{}, defaultMinerNames...), extra...)
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.Contains(base, pattern) {
			return true
		}
	}
	return false
}

func looksLikeMinerArgs(args []string) bool {
	hasStratum := false
	hasAlgo := false
	hasPoolFlag := false
	for i, arg := range args {
		lower := strings.ToLower(arg)
		if strings.Contains(lower, "stratum+tcp://") || strings.Contains(lower, "stratum+ssl://") || strings.Contains(lower, "stratum://") {
			hasStratum = true
		}
		switch lower {
		case "--algo", "-a", "--algorithm":
			hasAlgo = true
		case "-o", "--url", "--pool", "--pool-url", "--stratum":
			hasPoolFlag = true
		}
		if hasAlgo || hasPoolFlag {
			if i > 0 {
				// flags are enough when a stratum endpoint is present
			}
		}
	}
	return hasStratum && (hasAlgo || hasPoolFlag)
}

type minerFlags struct {
	values     map[string]string
	positional []string
}

func parseFlagAliases(args []string, aliases map[string]string) minerFlags {
	result := minerFlags{values: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			result.positional = append(result.positional, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value := ""
		if eq := strings.Index(name, "="); eq >= 0 {
			value = name[eq+1:]
			name = name[:eq]
		}
		name = strings.ToLower(name)
		canonical, ok := aliases[name]
		if !ok {
			continue
		}
		if value == "" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		}
		if value != "" {
			result.values[canonical] = value
		}
	}
	return result
}

var minerAliases = map[string]string{
	"url": "pool", "pool": "pool", "pool-url": "pool", "o": "pool", "stratum": "pool", "server": "pool",
	"user": "wallet", "u": "wallet", "wallet": "wallet", "address": "wallet", "login": "wallet",
	"algo": "algo", "a": "algo", "algorithm": "algo", "coin": "algo",
	"config": "config", "c": "config", "config-file": "config",
	"http-port": "api", "api-port": "api", "api": "api",
}

func parseMinerArgs(m *Miner, args []string) {
	flags := parseFlagAliases(args, minerAliases)
	m.Pool = flags.values["pool"]
	m.Wallet = flags.values["wallet"]
	m.Algorithm = flags.values["algo"]
	if m.Pool == "" {
		for _, arg := range args {
			lower := strings.ToLower(arg)
			if strings.Contains(lower, "stratum+tcp://") || strings.Contains(lower, "stratum+ssl://") || strings.Contains(lower, "stratum://") {
				m.Pool = strings.Trim(arg, "\"'")
				break
			}
		}
	}
	if m.APIPort == 0 {
		if port, err := strconv.Atoi(strings.TrimSpace(flags.values["api"])); err == nil && port > 0 && port < 65536 {
			m.APIPort = port
		}
	}
}

// enrichFromXMRigConfig reads a JSON config referenced by -c/--config. XMRig
// is by far the most common CPU miner and its config carries pool, wallet,
// algo and the HTTP API port.
func enrichFromXMRigConfig(m *Miner, args []string) {
	flags := parseFlagAliases(args, minerAliases)
	path := flags.values["config"]
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 {
		return
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return
	}
	if m.APIPort == 0 {
		for _, section := range []string{"http", "api"} {
			if sec, ok := root[section].(map[string]any); ok {
				if port := anyInt(sec["port"]); port > 0 && port < 65536 {
					m.APIPort = port
				}
			}
		}
	}
	if pools, ok := root["pools"].([]any); ok && len(pools) > 0 {
		if pool, ok := pools[0].(map[string]any); ok {
			if m.Pool == "" {
				m.Pool = anyString(pool["url"])
			}
			if m.Wallet == "" {
				m.Wallet = anyString(pool["user"])
			}
			if m.Algorithm == "" {
				m.Algorithm = anyString(pool["algo"])
			}
		}
	}
	if m.Algorithm == "" {
		m.Algorithm = anyString(root["algo"])
	}
	if m.Pool == "" {
		m.Pool = anyString(root["pool"])
	}
	if m.Wallet == "" {
		m.Wallet = anyString(root["user"])
	}
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return ""
}

func anyInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(t))
		return i
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	}
	return 0
}

func gpuIndexesForPID(gpus []GPU, pid int) []int {
	var out []int
	for _, g := range gpus {
		for _, p := range g.Processes {
			if p.PID == pid {
				out = append(out, g.Index)
				break
			}
		}
	}
	return out
}
