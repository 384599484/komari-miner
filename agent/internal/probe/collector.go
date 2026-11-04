package probe

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Collector gathers one Snapshot per call. It is safe for concurrent use as
// long as Collect itself is called from a single goroutine.
type Collector struct {
	cfg  Config
	demo *demoState
}

func NewCollector(cfg Config) *Collector {
	c := &Collector{cfg: cfg}
	if cfg.Demo {
		c.demo = newDemoState()
	}
	return c
}

// Collect returns a fresh snapshot. Collector failures are reported as
// warnings instead of aborting the whole sample, so a rig with a broken
// nvidia-smi still reports CPU/RAM/miner process data.
func (c *Collector) Collect() Snapshot {
	started := time.Now()
	if c.cfg.Demo && c.demo != nil {
		s := c.demo.collect()
		s.aggregate()
		s.CollectMillis = time.Since(started).Milliseconds()
		s.Name = c.rigName(s.Hostname)
		return s
	}

	s := Snapshot{
		Time:     time.Now().UTC(),
		Demo:     false,
		Warnings: []string{},
	}
	sys, err := readSystem()
	if err != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("system metrics: %v", err))
	}
	s.System = sys
	s.Hostname = sys.Hostname
	if s.Hostname == "" {
		s.Hostname, _ = hostname()
	}
	s.Name = c.rigName(s.Hostname)

	gpus, gpuErr := readGPUs()
	if gpuErr != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("GPU telemetry: %v", gpuErr))
	}
	s.GPUs = gpus

	miners, minerErr := scanMiners(gpus, c.cfg.MinerPatterns)
	if minerErr != nil {
		s.Warnings = append(s.Warnings, fmt.Sprintf("mining process scan: %v", minerErr))
	}
	fillMinerHashrates(miners)
	s.Miners = miners

	s.aggregate()
	s.CollectMillis = time.Since(started).Milliseconds()
	return s
}

func (c *Collector) rigName(hostname string) string {
	if c.cfg.Name != "" {
		return c.cfg.Name
	}
	return hostname
}

func (s *Snapshot) aggregate() {
	if len(s.GPUs) > 0 {
		var totalUtil float64
		for _, g := range s.GPUs {
			totalUtil += g.Utilization
			s.TotalPower += g.Power
			if g.MemoryTemp > s.MaxGPUTemp {
				s.MaxGPUTemp = g.MemoryTemp
			}
			if g.Temperature > s.MaxGPUTemp {
				s.MaxGPUTemp = g.Temperature
			}
			if g.Temperature >= 85 {
				s.Alerts = append(s.Alerts, fmt.Sprintf("GPU #%d 温度 %d°C，超过安全阈值", g.Index, g.Temperature))
			}
			if g.Temperature >= 75 {
				s.Alerts = append(s.Alerts, fmt.Sprintf("GPU #%d 温度偏高 %d°C", g.Index, g.Temperature))
			}
			if g.MemoryTemp >= 95 {
				s.Alerts = append(s.Alerts, fmt.Sprintf("GPU #%d 显存温度 %d°C，请立即检查散热", g.Index, g.MemoryTemp))
			} else if g.MemoryTemp >= 85 {
				s.Alerts = append(s.Alerts, fmt.Sprintf("GPU #%d 显存温度偏高 %d°C", g.Index, g.MemoryTemp))
			}
		}
		s.AvgGPUUsage = round2(totalUtil / float64(len(s.GPUs)))
	}
	s.TotalPower = round2(s.TotalPower)

	s.TotalHashrate = 0
	for i := range s.Miners {
		s.TotalHashrate += s.Miners[i].Hashrate
		s.Miners[i].HashrateText = FormatHashrate(s.Miners[i].Hashrate)
		if s.Miners[i].Status == "" {
			s.Miners[i].Status = "running"
		}
	}
	s.TotalHashrate = round2(s.TotalHashrate)
	if len(s.Miners) == 0 {
		s.Alerts = append(s.Alerts, "未检测到挖矿进程")
	} else if s.TotalHashrate <= 0 {
		s.Alerts = append(s.Alerts, fmt.Sprintf("检测到 %d 个挖矿进程，但算力接口不可用", len(s.Miners)))
	}
	sort.SliceStable(s.Miners, func(i, j int) bool {
		if s.Miners[i].Name == s.Miners[j].Name {
			return s.Miners[i].PID < s.Miners[j].PID
		}
		return s.Miners[i].Name < s.Miners[j].Name
	})
	s.Alerts = dedupeStrings(s.Alerts)
	if len(s.Warnings) == 0 {
		s.Warnings = nil
	}
}

func (s Snapshot) History() HistoryPoint {
	return HistoryPoint{
		Time:          s.Time,
		TotalHashrate: s.TotalHashrate,
		AvgGPUUsage:   s.AvgGPUUsage,
		MaxGPUTemp:    s.MaxGPUTemp,
		Miners:        len(s.Miners),
		TotalPower:    s.TotalPower,
	}
}

// GPUNameSummary builds the short GPU model summary uploaded as basic info.
func (s Snapshot) GPUNameSummary() string {
	if len(s.GPUs) == 0 {
		return ""
	}
	counts := make(map[string]int)
	order := make([]string, 0, len(s.GPUs))
	for _, g := range s.GPUs {
		if _, ok := counts[g.Name]; !ok {
			order = append(order, g.Name)
		}
		counts[g.Name]++
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, fmt.Sprintf("%dx %s", counts[name], name))
	}
	out := strings.Join(parts, ", ")
	if len(out) > 100 {
		out = out[:97] + "..."
	}
	return out
}

func (s Snapshot) OSName() string {
	if s.System.OS != "" {
		return s.System.OS
	}
	return runtime.GOOS
}

func round2(v float64) float64 {
	if v != v { // NaN
		return 0
	}
	return float64(int64(v*100+0.5)) / 100
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
