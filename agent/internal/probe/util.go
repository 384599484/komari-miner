package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func hostname() (string, error) {
	return os.Hostname()
}

// FormatHashrate renders a normalized H/s value with SI units.
func FormatHashrate(h float64) string {
	if h <= 0 {
		return "0 H/s"
	}
	units := []string{"H/s", "kH/s", "MH/s", "GH/s", "TH/s", "PH/s"}
	i := 0
	for h >= 1000 && i < len(units)-1 {
		h /= 1000
		i++
	}
	switch {
	case h >= 100:
		return fmt.Sprintf("%.0f %s", h, units[i])
	case h >= 10:
		return fmt.Sprintf("%.1f %s", h, units[i])
	default:
		return fmt.Sprintf("%.2f %s", h, units[i])
	}
}

// FormatBytes renders a byte count with binary units.
func FormatBytes(v int64) string {
	if v <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	f := float64(v)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if f >= 100 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	if f >= 10 {
		return fmt.Sprintf("%.1f %s", f, units[i])
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

// FormatDuration renders seconds as a compact d/h/m/s string.
func FormatDuration(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// parseFloat is deliberately permissive because vendor tools emit values such
// as "[N/A]", "82 %" or "123.5 W".
func parseFloat(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" || strings.Contains(strings.ToLower(v), "n/a") || strings.Contains(v, "not supported") {
		return 0
	}
	var b strings.Builder
	dot := false
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '.' && !dot {
			dot = true
			b.WriteRune(r)
		}
	}
	f, _ := strconv.ParseFloat(b.String(), 64)
	return f
}

func parseInt(v string) int {
	f := parseFloat(v)
	if f > float64(int(^uint(0)>>1)) {
		return 0
	}
	return int(f)
}

// commandTimeout is the budget for one external telemetry command. Slow
// virtualized rigs or rocm-smi on many cards can need more than the default.
func commandTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("PROBE_COMMAND_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Second {
			return d
		}
		if secs, err := strconv.Atoi(raw); err == nil && secs >= 1 {
			return time.Duration(secs) * time.Second
		}
	}
	return 6 * time.Second
}

func commandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commandTimeout())
}

// firstNumber recursively extracts the first finite number from JSON shapes
// such as 1234.5 or [1234.5, 1200.0, 1180.0] or {"total": 1234.5}.
func firstNumber(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case []any:
		for _, item := range t {
			if f := firstNumber(item); f > 0 {
				return f
			}
		}
	case map[string]any:
		for _, key := range []string{"total", "hashrate", "value", "current", "hashrate_1m", "hashrate_10m", "hashrate_15m"} {
			if child, ok := t[key]; ok {
				if f := firstNumber(child); f > 0 {
					return f
				}
			}
		}
	}
	return 0
}
