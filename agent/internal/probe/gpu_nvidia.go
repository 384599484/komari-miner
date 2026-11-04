package probe

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var nvidiaFullFields = []string{
	"index", "uuid", "name", "memory.total", "memory.used",
	"utilization.gpu", "temperature.gpu", "temperature.memory",
	"power.draw", "power.limit", "fan.speed",
	"clocks.current.graphics", "clocks.current.memory",
}

var nvidiaBaseFields = []string{
	"index", "uuid", "name", "memory.total", "memory.used",
	"utilization.gpu", "temperature.gpu",
}

const mib = 1024 * 1024

// readGPUs tries NVIDIA first (the common case on mining rigs), then falls
// back to the platform-specific vendor tool.
func readGPUs() ([]GPU, error) {
	gpus, ok, err := queryNvidiaSMI()
	if ok {
		attachNvidiaComputeApps(gpus)
		return gpus, err
	}
	other, otherErr := queryOtherGPUs()
	if len(other) > 0 {
		return other, nil
	}
	if err != nil && !errors.Is(err, errNoGPU) {
		if otherErr != nil {
			return nil, fmt.Errorf("%v; %v", err, otherErr)
		}
		return nil, err
	}
	return nil, otherErr
}

var errNoGPU = errors.New("no GPU telemetry source available")

func queryNvidiaSMI() ([]GPU, bool, error) {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, false, errNoGPU
	}
	out, err := runCommand("nvidia-smi", "--query-gpu="+strings.Join(nvidiaFullFields, ","), "--format=csv,noheader,nounits")
	gpus, parseErr := parseNvidiaRows(out, nvidiaFullFields)
	if err != nil || parseErr != nil || len(gpus) == 0 {
		// Older drivers do not support temperature.memory / fan.speed on every
		// SKU. Retry with the smallest portable field set before giving up.
		out, err = runCommand("nvidia-smi", "--query-gpu="+strings.Join(nvidiaBaseFields, ","), "--format=csv,noheader,nounits")
		gpus, parseErr = parseNvidiaRows(out, nvidiaBaseFields)
		if err != nil {
			return nil, false, fmt.Errorf("nvidia-smi: %w", err)
		}
		if parseErr != nil {
			return nil, false, fmt.Errorf("parse nvidia-smi: %w", parseErr)
		}
	}
	if len(gpus) == 0 {
		return nil, false, errNoGPU
	}
	for i := range gpus {
		gpus[i].Vendor = "NVIDIA"
		if gpus[i].Index < 0 {
			gpus[i].Index = i
		}
	}
	return gpus, true, nil
}

func parseNvidiaRows(out string, fields []string) ([]GPU, error) {
	r := csv.NewReader(strings.NewReader(out))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	gpus := make([]GPU, 0, len(records))
	for rowIndex, rec := range records {
		if len(rec) == 0 || strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue
		}
		// A row shorter than the index/name columns is garbage; longer rows are
		// fine because the optional tail is simply ignored.
		if len(rec) < 3 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			idx = rowIndex
		}
		field := func(i int) string {
			if i < len(rec) {
				return rec[i]
			}
			return ""
		}
		g := GPU{
			Index:       idx,
			UUID:        strings.TrimSpace(field(1)),
			Name:        strings.TrimSpace(field(2)),
			MemoryTotal: int64(parseFloat(field(3))) * mib,
			MemoryUsed:  int64(parseFloat(field(4))) * mib,
			Utilization: parseFloat(field(5)),
			Temperature: parseInt(field(6)),
		}
		if len(rec) >= 9 {
			g.MemoryTemp = parseInt(rec[7])
			g.Power = parseFloat(rec[8])
		}
		if len(rec) >= 11 {
			g.PowerLimit = parseFloat(rec[9])
			g.FanSpeed = parseInt(rec[10])
		}
		if len(rec) >= 13 {
			g.ClockMHz = parseInt(rec[11])
			g.MemoryClockMHz = parseInt(rec[12])
		}
		if g.Name == "" {
			g.Name = fmt.Sprintf("GPU %d", idx)
		}
		gpus = append(gpus, g)
	}
	if len(gpus) == 0 {
		return nil, errors.New("no GPU rows")
	}
	return gpus, nil
}

// attachNvidiaComputeApps maps compute PIDs to GPU indices. It is best effort:
// MIG, containers and some older drivers simply return nothing.
func attachNvidiaComputeApps(gpus []GPU) {
	out, err := runCommand("nvidia-smi", "--query-compute-apps=pid,process_name,gpu_uuid,used_memory", "--format=csv,noheader,nounits")
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}
	byUUID := make(map[string]*GPU, len(gpus))
	for i := range gpus {
		if gpus[i].UUID != "" {
			byUUID[gpus[i].UUID] = &gpus[i]
		}
	}
	r := csv.NewReader(strings.NewReader(out))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return
	}
	for _, rec := range records {
		if len(rec) < 3 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil || pid <= 0 {
			continue
		}
		name := strings.TrimSpace(rec[1])
		gpu := byUUID[strings.TrimSpace(rec[2])]
		if gpu == nil {
			continue
		}
		mem := int64(0)
		if len(rec) > 3 {
			mem = int64(parseFloat(rec[3])) * mib
		}
		gpu.Processes = append(gpu.Processes, GPUProcess{PID: pid, Name: name, MemoryBytes: mem})
	}
}

func runCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s timed out after %s", name, commandTimeout())
	}
	return string(out), err
}
