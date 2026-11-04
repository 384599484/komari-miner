// Package probe implements a mining-rig monitoring probe. It collects GPU
// telemetry, detects mining processes and can either serve its own dashboard
// or report to a Komari server using the v2 agent protocol.
package probe

import (
	"time"
)

// Version is the probe build version.
const Version = "0.1.0"

// Config is the runtime configuration of the probe.
type Config struct {
	Name           string        `json:"name"`
	Listen         string        `json:"listen"`
	Interval       time.Duration `json:"-"`
	IntervalSecond int           `json:"interval"`
	KomariURL      string        `json:"komari_url"`
	Token          string        `json:"token"`
	Demo           bool          `json:"demo"`
	DisableDash    bool          `json:"disable_dashboard"`
	MinerPatterns  []string      `json:"miner_patterns"`
	Version        string        `json:"-"`
}

// System carries the general host metrics expected by the Komari protocol.
type System struct {
	CPUName      string  `json:"cpu_name"`
	CPUCores     int     `json:"cpu_cores"`
	Arch         string  `json:"arch"`
	CPUUsage     float64 `json:"cpu_usage"`
	Hostname     string  `json:"hostname"`
	OS           string  `json:"os"`
	Kernel       string  `json:"kernel,omitempty"`
	RAMTotal     int64   `json:"ram_total"`
	RAMUsed      int64   `json:"ram_used"`
	SwapTotal    int64   `json:"swap_total"`
	SwapUsed     int64   `json:"swap_used"`
	DiskTotal    int64   `json:"disk_total"`
	DiskUsed     int64   `json:"disk_used"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
	NetUp        int64   `json:"net_up"`
	NetDown      int64   `json:"net_down"`
	NetTotalUp   int64   `json:"net_total_up"`
	NetTotalDown int64   `json:"net_total_down"`
	TCP          int     `json:"tcp"`
	UDP          int     `json:"udp"`
	Uptime       int64   `json:"uptime"`
	Processes    int     `json:"processes"`
}

// GPUProcess is a compute process attached to a GPU, as seen by nvidia-smi.
type GPUProcess struct {
	PID         int    `json:"pid"`
	Name        string `json:"name"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// GPU is one physical GPU on the rig.
type GPU struct {
	Index          int          `json:"index"`
	UUID           string       `json:"uuid,omitempty"`
	Name           string       `json:"name"`
	MemoryTotal    int64        `json:"memory_total"`
	MemoryUsed     int64        `json:"memory_used"`
	Utilization    float64      `json:"utilization"`
	Temperature    int          `json:"temperature"`
	MemoryTemp     int          `json:"memory_temperature,omitempty"`
	Power          float64      `json:"power,omitempty"`
	PowerLimit     float64      `json:"power_limit,omitempty"`
	FanSpeed       int          `json:"fan_speed,omitempty"`
	ClockMHz       int          `json:"clock_mhz,omitempty"`
	MemoryClockMHz int          `json:"memory_clock_mhz,omitempty"`
	Vendor         string       `json:"vendor,omitempty"`
	Processes      []GPUProcess `json:"processes,omitempty"`
}

// Miner describes a detected mining process.
type Miner struct {
	PID            int       `json:"pid"`
	Name           string    `json:"name"`
	Exe            string    `json:"exe,omitempty"`
	User           string    `json:"user,omitempty"`
	Algorithm      string    `json:"algorithm,omitempty"`
	Hashrate       float64   `json:"hashrate,omitempty"`
	HashrateText   string    `json:"hashrate_text,omitempty"`
	AcceptedShares int64     `json:"accepted_shares,omitempty"`
	RejectedShares int64     `json:"rejected_shares,omitempty"`
	Pool           string    `json:"pool,omitempty"`
	Wallet         string    `json:"wallet,omitempty"`
	GPUs           []int     `json:"gpus,omitempty"`
	CPUPercent     float64   `json:"cpu_percent,omitempty"`
	MemoryBytes    int64     `json:"memory_bytes,omitempty"`
	Uptime         int64     `json:"uptime,omitempty"`
	StartTime      time.Time `json:"start_time,omitempty"`
	Command        string    `json:"command,omitempty"`
	Status         string    `json:"status,omitempty"`

	// APIPort is the local miner HTTP API port used to fetch hashrate/share
	// counters. It is internal and not exposed in JSON.
	APIPort int `json:"-"`
}

// Snapshot is a complete observation of the rig at one point in time.
type Snapshot struct {
	Time          time.Time `json:"time"`
	Name          string    `json:"name"`
	Hostname      string    `json:"hostname"`
	Demo          bool      `json:"demo"`
	System        System    `json:"system"`
	GPUs          []GPU     `json:"gpus"`
	Miners        []Miner   `json:"miners"`
	TotalHashrate float64   `json:"total_hashrate"`
	TotalPower    float64   `json:"total_power"`
	AvgGPUUsage   float64   `json:"avg_gpu_usage"`
	MaxGPUTemp    int       `json:"max_gpu_temp"`
	Alerts        []string  `json:"alerts"`
	Warnings      []string  `json:"warnings,omitempty"`
	CollectMillis int64     `json:"collect_ms"`
}

// HistoryPoint is a compact sample used by the dashboard sparklines.
type HistoryPoint struct {
	Time          time.Time `json:"time"`
	TotalHashrate float64   `json:"total_hashrate"`
	AvgGPUUsage   float64   `json:"avg_gpu_usage"`
	MaxGPUTemp    int       `json:"max_gpu_temp"`
	Miners        int       `json:"miners"`
	TotalPower    float64   `json:"total_power"`
}
