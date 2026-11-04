//go:build windows

package probe

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

type windowsSystemPayload struct {
	CPUName   string  `json:"CPUName"`
	Cores     int     `json:"Cores"`
	CPUUsage  float64 `json:"CPUUsage"`
	MemTotal  int64   `json:"MemTotal"`
	MemFree   int64   `json:"MemFree"`
	SwapTotal int64   `json:"SwapTotal"`
	SwapUsed  int64   `json:"SwapUsed"`
	DiskTotal int64   `json:"DiskTotal"`
	DiskFree  int64   `json:"DiskFree"`
	NetIn     int64   `json:"NetIn"`
	NetOut    int64   `json:"NetOut"`
	Uptime    int64   `json:"Uptime"`
	Processes int     `json:"Processes"`
	TCP       int     `json:"TCP"`
	UDP       int     `json:"UDP"`
	OSVersion string  `json:"OSVersion"`
	Hostname  string  `json:"Hostname"`
}

const windowsSystemScript = `
$ErrorActionPreference = "SilentlyContinue"
$os = Get-CimInstance Win32_OperatingSystem
$cs = Get-CimInstance Win32_ComputerSystem
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$disk = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='C:'"
if (-not $disk) { $disk = Get-CimInstance Win32_LogicalDisk | Select-Object -First 1 }
$net = Get-NetAdapterStatistics | Measure-Object -Property ReceivedBytes,SentBytes -Sum
$tcp = (Get-NetTCPConnection | Measure-Object).Count
$udp = (Get-NetUDPEndpoint | Measure-Object).Count
$page = Get-CimInstance Win32_PageFileUsage | Select-Object -First 1
[PSCustomObject]@{
  CPUName = $cpu.Name
  Cores = [int]$cs.NumberOfLogicalProcessors
  CPUUsage = [double]$cpu.LoadPercentage
  MemTotal = [long]$cs.TotalPhysicalMemory
  MemFree = [long]$os.FreePhysicalMemory * 1024
  SwapTotal = [long]$page.AllocatedBaseSize * 1MB
  SwapUsed = [long]$page.CurrentUsage * 1MB
  DiskTotal = [long]$disk.Size
  DiskFree = [long]$disk.FreeSpace
  NetIn = [long]$net.Sum[0]
  NetOut = [long]$net.Sum[1]
  Uptime = [long]((Get-Date) - $os.LastBootUpTime).TotalSeconds
  Processes = (Get-Process | Measure-Object).Count
  TCP = [int]$tcp
  UDP = [int]$udp
  OSVersion = $os.Version
  Hostname = $env:COMPUTERNAME
} | ConvertTo-Json -Compress
`

func readSystem() (System, error) {
	out, err := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", windowsSystemScript)
	if err != nil {
		return System{OS: "windows", Arch: runtime.GOARCH, CPUCores: runtime.NumCPU()}, fmt.Errorf("powershell: %w", err)
	}
	var p windowsSystemPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &p); err != nil {
		return System{OS: "windows", Arch: runtime.GOARCH, CPUCores: runtime.NumCPU()}, fmt.Errorf("parse system json: %w", err)
	}
	s := System{
		CPUName:   strings.TrimSpace(p.CPUName),
		CPUCores:  p.Cores,
		Arch:      runtime.GOARCH,
		CPUUsage:  round2(p.CPUUsage),
		Hostname:  p.Hostname,
		OS:        "windows",
		Kernel:    p.OSVersion,
		RAMTotal:  p.MemTotal,
		RAMUsed:   p.MemTotal - p.MemFree,
		SwapTotal: p.SwapTotal,
		SwapUsed:  p.SwapUsed,
		DiskTotal: p.DiskTotal,
		DiskUsed:  p.DiskTotal - p.DiskFree,
		Uptime:    p.Uptime,
		Processes: p.Processes,
		TCP:       p.TCP,
		UDP:       p.UDP,
	}
	if s.CPUCores == 0 {
		s.CPUCores = runtime.NumCPU()
	}
	return s, nil
}
