package probe

import (
	"strings"
	"testing"
	"time"
)

func TestDemoCollector(t *testing.T) {
	c := NewCollector(Config{Demo: true, Interval: 2 * time.Second, Name: "test-rig"})
	s := c.Collect()
	if s.Name != "test-rig" {
		t.Fatalf("name = %q", s.Name)
	}
	if len(s.GPUs) != 8 {
		t.Fatalf("gpus = %d", len(s.GPUs))
	}
	if len(s.Miners) != 3 {
		t.Fatalf("miners = %d", len(s.Miners))
	}
	if s.TotalHashrate <= 0 || s.TotalPower <= 0 {
		t.Fatalf("aggregates not populated: %#v", s)
	}
	for _, alert := range s.Alerts {
		if strings.Contains(alert, "算力接口不可用") {
			t.Fatalf("demo hashrate incorrectly reported unavailable: %v", s.Alerts)
		}
	}
	for _, m := range s.Miners {
		if m.HashrateText == "" || m.HashrateText == "0 H/s" {
			t.Fatalf("miner hashrate text not set: %#v", m)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[int64]string{
		0:     "-",
		45:    "45s",
		125:   "2m 5s",
		3700:  "1h 1m",
		90000: "1d 1h",
	}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(0); got != "0 B" {
		t.Errorf("0 = %q", got)
	}
	if got := FormatBytes(1024 * 1024 * 1024); got != "1.00 GiB" {
		t.Errorf("1GiB = %q", got)
	}
}

func TestGPUNameSummary(t *testing.T) {
	s := Snapshot{GPUs: []GPU{{Name: "RTX 4090"}, {Name: "RTX 4090"}, {Name: "RTX 3090"}}}
	got := s.GPUNameSummary()
	if got != "2x RTX 4090, 1x RTX 3090" {
		t.Fatalf("summary = %q", got)
	}
}
