//go:build linux

package probe

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var cardIndexRe = regexp.MustCompile(`(?i)card\s*(\d+)`)

// queryOtherGPUs tries AMD's rocm-smi on Linux.
func queryOtherGPUs() ([]GPU, error) {
	if _, err := exec.LookPath("rocm-smi"); err != nil {
		return nil, errNoGPU
	}
	out, err := runCommand("rocm-smi",
		"--showproductname", "--showmeminfo", "vram", "--showuse",
		"--showtemp", "--showpower", "--showfan", "--json")
	if err != nil {
		return nil, fmt.Errorf("rocm-smi: %w", err)
	}
	return parseROCmOutput(out)
}

// parseROCmOutput decodes the JSON document produced by rocm-smi --json.
func parseROCmOutput(out string) ([]GPU, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		return nil, fmt.Errorf("parse rocm-smi: %w", err)
	}
	type card struct {
		index int
		m     map[string]any
	}
	cards := make([]card, 0, len(root))
	for key, raw := range root {
		target, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		idx := 0
		if m := cardIndexRe.FindStringSubmatch(key); len(m) == 2 {
			idx, _ = strconv.Atoi(m[1])
		}
		cards = append(cards, card{index: idx, m: target})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].index < cards[j].index })

	gpus := make([]GPU, 0, len(cards))
	for _, c := range cards {
		name := firstStringValue(c.m, "Card series", "Card SKU", "Product Name", "Card model", "GPU ID")
		if name == "" {
			name = fmt.Sprintf("AMD GPU %d", c.index)
		}
		g := GPU{
			Index:  c.index,
			Name:   name,
			Vendor: "AMD",
			MemoryTotal: int64(firstNumberValue(c.m,
				"VRAM Total Memory (B)", "VRAM Total Memory", "Total Memory (B)")),
			MemoryUsed: int64(firstNumberValue(c.m,
				"VRAM Total Used Memory (B)", "VRAM Total Used Memory", "Used Memory (B)")),
			Utilization: firstNumberValue(c.m, "GPU use (%)", "GPU use", "GPU Utilization"),
			Temperature: int(firstNumberValue(c.m,
				"Temperature (Sensor edge) (C)", "Temperature (Sensor junction) (C)",
				"Temperature (Sensor memory) (C)", "Temperature (C)")),
			Power:      firstNumberValue(c.m, "Current Power (W)", "Average Power (W)", "Power (W)"),
			PowerLimit: firstNumberValue(c.m, "Max Power (W)", "Power Cap (W)"),
			FanSpeed:   int(firstNumberValue(c.m, "Fan speed (%)", "Fan Speed")),
			ClockMHz:   int(firstNumberValue(c.m, "GPU Clock (MHz)", "sclk clock speed:")),
		}
		gpus = append(gpus, g)
	}
	if len(gpus) == 0 {
		return nil, errNoGPU
	}
	return gpus, nil
}

// findValue recursively looks up a key by case-insensitive substring. rocm-smi
// changes key names and nesting between releases, so rigid structs are fragile.
func findValue(v any, needles ...string) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			for _, needle := range needles {
				if strings.Contains(lk, strings.ToLower(needle)) {
					return child, true
				}
			}
		}
		for _, child := range t {
			if found, ok := findValue(child, needles...); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range t {
			if found, ok := findValue(child, needles...); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func firstNumberValue(m map[string]any, needles ...string) float64 {
	for _, needle := range needles {
		if v, ok := findValue(m, needle); ok {
			switch t := v.(type) {
			case string:
				return parseFloat(t)
			case float64:
				return t
			}
		}
	}
	return 0
}

func firstStringValue(m map[string]any, needles ...string) string {
	for _, needle := range needles {
		if v, ok := findValue(m, needle); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
