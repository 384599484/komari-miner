//go:build windows

package probe

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type windowsProcess struct {
	ProcessID int    `json:"ProcessId"`
	Name      string `json:"Name"`
	Exe       string `json:"Exe"`
	Command   string `json:"CommandLine"`
	CreatedAt string `json:"CreatedAt"`
}

const windowsProcessScript = `
$ErrorActionPreference = "SilentlyContinue"
Get-CimInstance Win32_Process | ForEach-Object {
  [PSCustomObject]@{
    ProcessId = $_.ProcessId
    Name = $_.Name
    Exe = $_.ExecutablePath
    CommandLine = $_.CommandLine
    CreatedAt = $(if ($_.CreationDate) { $_.CreationDate.ToString("o") } else { "" })
  }
} | ConvertTo-Json -Compress
`

func scanMiners(gpus []GPU, extraPatterns []string) ([]Miner, error) {
	ctx, cancel := commandContext()
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", windowsProcessScript)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}
	var processes []windowsProcess
	if strings.HasPrefix(text, "{") {
		var one windowsProcess
		if err := json.Unmarshal([]byte(text), &one); err != nil {
			return nil, err
		}
		processes = []windowsProcess{one}
	} else if err := json.Unmarshal([]byte(text), &processes); err != nil {
		return nil, err
	}

	miners := make([]Miner, 0, 4)
	for _, p := range processes {
		if p.ProcessID <= 1 {
			continue
		}
		args := splitWindowsArgs(p.Command)
		exe := p.Exe
		if exe == "" && len(args) > 0 {
			exe = strings.Trim(args[0], "\"")
		}
		if !matchMinerName(p.Name, extraPatterns) && !matchMinerName(exe, extraPatterns) && !looksLikeMinerArgs(args) {
			continue
		}
		start, _ := time.Parse(time.RFC3339Nano, p.CreatedAt)
		uptime := int64(0)
		if !start.IsZero() {
			uptime = int64(time.Since(start).Seconds())
		}
		candidate := processCandidate{
			PID:       p.ProcessID,
			Name:      p.Name,
			Exe:       exe,
			Command:   p.Command,
			Args:      args,
			Uptime:    uptime,
			StartTime: start,
		}
		if m, ok := buildMiner(candidate, gpus, extraPatterns); ok {
			miners = append(miners, m)
		}
	}
	sort.Slice(miners, func(i, j int) bool { return miners[i].PID < miners[j].PID })
	return miners, nil
}

func splitWindowsArgs(command string) []string {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	var args []string
	var current strings.Builder
	inQuote := false
	for _, r := range command {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func windowsProcessCount() int {
	ctx, cancel := commandContext()
	defer cancel()
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-Process | Measure-Object).Count").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}
