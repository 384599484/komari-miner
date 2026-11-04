package probe

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// demoState produces realistic-but-synthetic telemetry so the dashboard and
// the Komari report path can be exercised on a workstation without GPUs.
type demoState struct {
	mu      sync.Mutex
	rnd     *rand.Rand
	started time.Time
	tick    int
	gpus    []GPU
	miners  []Miner
}

func newDemoState() *demoState {
	now := time.Now()
	d := &demoState{
		rnd:     rand.New(rand.NewSource(now.UnixNano())),
		started: now,
	}
	models := []struct {
		name  string
		total int64
		limit float64
		clock int
	}{
		{"NVIDIA GeForce RTX 4090", 24 * 1024 * mib, 450, 2520},
		{"NVIDIA GeForce RTX 4090", 24 * 1024 * mib, 450, 2520},
		{"NVIDIA GeForce RTX 4090", 24 * 1024 * mib, 450, 2520},
		{"NVIDIA GeForce RTX 4090", 24 * 1024 * mib, 450, 2520},
		{"NVIDIA GeForce RTX 3090", 24 * 1024 * mib, 350, 1920},
		{"NVIDIA GeForce RTX 3090", 24 * 1024 * mib, 350, 1920},
		{"NVIDIA GeForce RTX 3090", 24 * 1024 * mib, 350, 1920},
		{"NVIDIA GeForce RTX 3090", 24 * 1024 * mib, 350, 1920},
	}
	for i, model := range models {
		temp := 58 + d.rnd.Intn(8)
		if i >= 4 {
			temp += 8
		}
		d.gpus = append(d.gpus, GPU{
			Index:          i,
			UUID:           fmt.Sprintf("GPU-demo-%04d", i),
			Name:           model.name,
			MemoryTotal:    model.total,
			MemoryUsed:     int64(float64(model.total) * (0.86 + d.rnd.Float64()*0.09)),
			Utilization:    round2(90 + d.rnd.Float64()*8),
			Temperature:    temp,
			MemoryTemp:     temp + 10 + d.rnd.Intn(4),
			Power:          model.limit * (0.80 + d.rnd.Float64()*0.15),
			PowerLimit:     model.limit,
			FanSpeed:       50 + d.rnd.Intn(28),
			ClockMHz:       model.clock - d.rnd.Intn(120),
			MemoryClockMHz: 10501,
			Vendor:         "NVIDIA",
		})
	}
	base := now.Add(-76 * time.Hour)
	d.miners = []Miner{
		{
			PID: 4211, Name: "t-rex", Exe: "/opt/miners/t-rex/t-rex", User: "miner",
			Algorithm: "kawpow", Hashrate: 62.4e6, AcceptedShares: 12840, RejectedShares: 7,
			Pool:   "stratum+tcp://pool.woolypooly.com:3140",
			Wallet: "0x7f3a9c2d84b1e6f05a8c3d7e9b2f14a6c8d0e3b1",
			GPUs:   []int{0, 1, 2, 3}, CPUPercent: 11.2, MemoryBytes: 2_100_000_000,
			Uptime:  int64(now.Sub(base).Seconds()),
			Command: "/opt/miners/t-rex/t-rex -a kawpow -o stratum+tcp://pool.woolypooly.com:3140 -u 0x7f3a...e3b1 -p x --gpu-report-interval 30",
			Status:  "running",
		},
		{
			PID: 4388, Name: "lolMiner", Exe: "/opt/miners/lolMiner/lolMiner", User: "miner",
			Algorithm: "etchash", Hashrate: 247.8e6, AcceptedShares: 31502, RejectedShares: 21,
			Pool:   "stratum+ssl://etc.2miners.com:1010",
			Wallet: "0x91ab7d3e5f2c8a4b6d0f1e9c7a3b5d2f8e4c6a10",
			GPUs:   []int{4, 5, 6, 7}, CPUPercent: 8.7, MemoryBytes: 1_600_000_000,
			Uptime:  int64(now.Sub(base.Add(2 * time.Hour)).Seconds()),
			Command: "/opt/miners/lolMiner/lolMiner --algo ETCHASH --pool stratum+ssl://etc.2miners.com:1010 --user 0x91ab...6a10 --dualmode off",
			Status:  "running",
		},
		{
			PID: 4010, Name: "xmrig", Exe: "/opt/miners/xmrig/xmrig", User: "miner",
			Algorithm: "rx/0", Hashrate: 12.4e3, AcceptedShares: 842, RejectedShares: 0,
			Pool:       "stratum+tcp://pool.supportxmr.com:3333",
			Wallet:     "48b3f9c2a7e14d6f0b8c5a2e9d7f1c4b6a8e0d2f5c7b9a1e3d5f7c9b1a3e5d7f9c1b3a",
			CPUPercent: 712, MemoryBytes: 1_240_000_000,
			Uptime:  int64(now.Sub(base.Add(26 * time.Hour)).Seconds()),
			Command: "/opt/miners/xmrig/xmrig -c /opt/miners/xmrig/config.json --http-port 18080",
			Status:  "running",
		},
	}
	for i := range d.gpus {
		for _, m := range d.miners {
			for _, gpuIndex := range m.GPUs {
				if gpuIndex == i {
					d.gpus[i].Processes = append(d.gpus[i].Processes, GPUProcess{
						PID: m.PID, Name: m.Name, MemoryBytes: 2_100_000_000,
					})
				}
			}
		}
	}
	return d
}

func (d *demoState) collect() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tick++
	now := time.Now().UTC()
	uptime := int64(17*86400) + int64(now.Sub(d.started).Seconds())

	for i := range d.gpus {
		g := &d.gpus[i]
		g.Utilization = clamp(g.Utilization+float64(d.rnd.Intn(7)-3), 76, 100)
		g.Temperature = int(clamp(float64(g.Temperature)+float64(d.rnd.Intn(5)-2), 58, 84))
		g.MemoryTemp = g.Temperature + 10 + d.rnd.Intn(5)
		g.FanSpeed = int(clamp(float64(g.FanSpeed)+float64(d.rnd.Intn(5)-2), 38, 92))
		g.Power = clamp(g.Power+float64(d.rnd.Intn(21)-10), g.PowerLimit*0.62, g.PowerLimit*1.02)
		g.MemoryUsed = int64(float64(g.MemoryTotal) * clamp(0.86+float64(d.rnd.Intn(10))/100, 0.84, 0.97))
	}
	for i := range d.miners {
		m := &d.miners[i]
		m.Uptime += 5
		m.AcceptedShares += int64(d.rnd.Intn(3))
		switch m.Name {
		case "t-rex":
			m.Hashrate = clamp(m.Hashrate+float64(d.rnd.Intn(900000)-450000), 58e6, 66e6)
		case "lolMiner":
			m.Hashrate = clamp(m.Hashrate+float64(d.rnd.Intn(2500000)-1250000), 235e6, 258e6)
		case "xmrig":
			m.Hashrate = clamp(m.Hashrate+float64(d.rnd.Intn(600)-300), 11e3, 14e3)
		}
	}

	s := Snapshot{
		Time:     now,
		Name:     "rig-demo-01",
		Hostname: "rig-demo-01",
		Demo:     true,
		System: System{
			CPUName:      "AMD Ryzen 9 7950X 16-Core Processor",
			CPUCores:     32,
			Arch:         "amd64",
			CPUUsage:     round2(18 + d.rnd.Float64()*14),
			Hostname:     "rig-demo-01",
			OS:           "linux",
			Kernel:       "6.8.0-45-generic",
			RAMTotal:     128 * 1024 * mib,
			RAMUsed:      int64(float64(128*1024*mib) * (0.41 + d.rnd.Float64()*0.06)),
			SwapTotal:    8 * 1024 * mib,
			SwapUsed:     0,
			DiskTotal:    2 * 1024 * 1024 * mib,
			DiskUsed:     2 * 1024 * 1024 * mib * 68 / 100,
			Load1:        3.2 + d.rnd.Float64(),
			Load5:        3.0 + d.rnd.Float64(),
			Load15:       2.8 + d.rnd.Float64(),
			NetUp:        int64(8_000_000 + d.rnd.Intn(9_000_000)),
			NetDown:      int64(12_000_000 + d.rnd.Intn(14_000_000)),
			NetTotalUp:   2_840_000_000_000,
			NetTotalDown: 9_120_000_000_000,
			TCP:          238 + d.rnd.Intn(20),
			UDP:          64 + d.rnd.Intn(12),
			Uptime:       uptime,
			Processes:    386 + d.rnd.Intn(12),
		},
		GPUs:   append([]GPU(nil), d.gpus...),
		Miners: append([]Miner(nil), d.miners...),
	}
	return s
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
