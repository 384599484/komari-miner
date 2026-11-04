package metricstore

import (
	"testing"
	"time"

	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func TestReportMetricPointsIncludesMining(t *testing.T) {
	now := time.Now().UTC()
	report := v2.Report{
		UUID:      "rig-a",
		UpdatedAt: now,
		GPU: &v2.GPUDetailReport{
			Count:        1,
			AverageUsage: 91,
			DetailedInfo: []v2.GPUDeviceInfo{{Index: 0, Name: "RTX 4090", Utilization: 91, Temperature: 70}},
		},
		Mining: &v2.MiningReport{
			MinerCount:    1,
			TotalHashrate: 62_000_000,
			TotalPower:    300,
			Miners: []v2.MinerProcess{{
				PID: 42, Name: "t-rex", Algorithm: "kawpow",
				Hashrate: 62_000_000, AcceptedShares: 10, Pool: "pool:3333",
			}},
		},
	}
	points := reportMetricPoints(report, 0, 0)
	find := func(name string, tags map[string]string) (float64, bool) {
		for _, point := range points {
			if point.MetricName != name || point.EntityID != "rig-a" {
				continue
			}
			if tags != nil && point.Tags["miner"] != tags["miner"] {
				continue
			}
			return point.Value, true
		}
		return 0, false
	}
	if v, ok := find(MetricMinerHashrate, nil); !ok || v != 62_000_000 {
		t.Fatalf("miner hashrate point missing: %v %v", v, ok)
	}
	if v, ok := find(MetricMinerCount, nil); !ok || v != 1 {
		t.Fatalf("miner count point missing: %v %v", v, ok)
	}
	if v, ok := find(MetricMinerPower, nil); !ok || v != 300 {
		t.Fatalf("miner power point missing: %v %v", v, ok)
	}
	if v, ok := find(MetricMinerProcessHash, map[string]string{"miner": "t-rex"}); !ok || v != 62_000_000 {
		t.Fatalf("per-process hashrate point missing: %v %v", v, ok)
	}
	if v, ok := find(MetricMinerProcessShares, map[string]string{"miner": "t-rex"}); !ok || v != 10 {
		t.Fatalf("per-process shares point missing: %v %v", v, ok)
	}
}
