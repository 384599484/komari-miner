//go:build darwin

package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// queryOtherGPUs uses system_profiler on macOS. Apple GPUs expose no
// utilization counters through this API, so those fields stay at zero; the
// probe is normally deployed on Linux mining rigs.
func queryOtherGPUs() ([]GPU, error) {
	out, err := runCommand("system_profiler", "SPDisplaysDataType", "-json")
	if err != nil {
		return nil, fmt.Errorf("system_profiler: %w", err)
	}
	var payload struct {
		Displays []struct {
			Name       string `json:"_name"`
			Model      string `json:"sppci_model"`
			Vendor     string `json:"sppci_vendor"`
			VRAM       string `json:"spdisplays_vram"`
			VRAMShared string `json:"spdisplays_vram_shared"`
			Cores      string `json:"sppci_cores"`
		} `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil, fmt.Errorf("parse system_profiler: %w", err)
	}
	gpus := make([]GPU, 0, len(payload.Displays))
	for i, d := range payload.Displays {
		name := strings.TrimSpace(d.Model)
		if name == "" {
			name = strings.TrimSpace(d.Name)
		}
		if name == "" {
			name = fmt.Sprintf("GPU %d", i)
		}
		mem := parseAppleVRAM(d.VRAM)
		if mem == 0 {
			mem = parseAppleVRAM(d.VRAMShared)
		}
		gpus = append(gpus, GPU{
			Index:       i,
			Name:        name,
			MemoryTotal: mem,
			Vendor:      strings.TrimSpace(d.Vendor),
		})
	}
	if len(gpus) == 0 {
		return nil, errors.New("no display GPU found")
	}
	return gpus, nil
}

func parseAppleVRAM(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	lower := strings.ToLower(s)
	mult := int64(1)
	switch {
	case strings.Contains(lower, "gb"):
		mult = 1024 * 1024 * 1024
	case strings.Contains(lower, "mb"):
		mult = 1024 * 1024
	}
	// "8 GB" -> 8, "Unified Memory" -> 0.
	var digits strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			digits.WriteRune(r)
		} else if digits.Len() > 0 {
			break
		}
	}
	f, err := strconv.ParseFloat(digits.String(), 64)
	if err != nil {
		return 0
	}
	return int64(f * float64(mult))
}
