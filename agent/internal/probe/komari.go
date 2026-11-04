package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	v2 "github.com/komari-monitor/komari/protocol/v2"
)

// KomariClient reports probe snapshots to a Komari server over the same v2
// JSON-RPC endpoint used by the official agent.
type KomariClient struct {
	baseURL string
	token   string
	http    *http.Client
	nextID  atomic.Int64
	// reportOnce disables the retry loop; used by tests and --once paths.
	reportOnce bool
}

func NewKomariClient(baseURL, token string) *KomariClient {
	return &KomariClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *KomariClient) enabled() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *KomariClient) endpoint() string {
	return c.baseURL + "/api/clients/v2/rpc?token=" + url.QueryEscape(c.token)
}

// ToReport converts a snapshot to the Komari v2 report payload.
func (s Snapshot) ToReport() v2.Report {
	rep := v2.Report{
		CPU: v2.CPUReport{
			Name:  s.System.CPUName,
			Cores: s.System.CPUCores,
			Arch:  s.System.Arch,
			Usage: s.System.CPUUsage,
		},
		Ram:         v2.RamReport{Total: s.System.RAMTotal, Used: s.System.RAMUsed},
		Swap:        v2.RamReport{Total: s.System.SwapTotal, Used: s.System.SwapUsed},
		Load:        v2.LoadReport{Load1: s.System.Load1, Load5: s.System.Load5, Load15: s.System.Load15},
		Disk:        v2.DiskReport{Total: s.System.DiskTotal, Used: s.System.DiskUsed},
		Network:     v2.NetworkReport{Up: s.System.NetUp, Down: s.System.NetDown, TotalUp: s.System.NetTotalUp, TotalDown: s.System.NetTotalDown},
		Connections: v2.ConnectionsReport{TCP: s.System.TCP, UDP: s.System.UDP},
		Uptime:      s.System.Uptime,
		Process:     s.System.Processes,
		Message:     strings.Join(s.Alerts, "; "),
	}
	if len(s.GPUs) > 0 {
		grep := &v2.GPUDetailReport{Count: len(s.GPUs), AverageUsage: s.AvgGPUUsage}
		grep.DetailedInfo = make([]v2.GPUDeviceInfo, 0, len(s.GPUs))
		for _, g := range s.GPUs {
			grep.DetailedInfo = append(grep.DetailedInfo, v2.GPUDeviceInfo{
				Index:             g.Index,
				UUID:              g.UUID,
				Name:              g.Name,
				MemoryTotal:       g.MemoryTotal,
				MemoryUsed:        g.MemoryUsed,
				Utilization:       g.Utilization,
				Temperature:       g.Temperature,
				MemoryTemperature: g.MemoryTemp,
				Power:             g.Power,
				PowerLimit:        g.PowerLimit,
				FanSpeed:          g.FanSpeed,
				ClockMHz:          g.ClockMHz,
				MemoryClockMHz:    g.MemoryClockMHz,
			})
		}
		rep.GPU = grep
	}
	mining := &v2.MiningReport{
		MinerCount:    len(s.Miners),
		TotalHashrate: s.TotalHashrate,
		TotalPower:    s.TotalPower,
		Alerts:        append([]string{}, s.Alerts...),
	}
	mining.Miners = make([]v2.MinerProcess, 0, len(s.Miners))
	for _, m := range s.Miners {
		mining.Miners = append(mining.Miners, v2.MinerProcess{
			PID:            m.PID,
			Name:           m.Name,
			User:           m.User,
			Algorithm:      m.Algorithm,
			Hashrate:       m.Hashrate,
			HashrateText:   m.HashrateText,
			AcceptedShares: m.AcceptedShares,
			RejectedShares: m.RejectedShares,
			Pool:           m.Pool,
			Wallet:         m.Wallet,
			GPUs:           append([]int{}, m.GPUs...),
			CPUPercent:     m.CPUPercent,
			MemoryBytes:    m.MemoryBytes,
			Uptime:         m.Uptime,
			Command:        m.Command,
			Exe:            m.Exe,
			Status:         m.Status,
		})
	}
	rep.Mining = mining
	return rep
}

func (c *KomariClient) send(ctx context.Context, method string, params any) error {
	req := v2.Request{
		JSONRPC: v2.Version,
		Method:  method,
		Params:  params,
		ID:      c.nextID.Add(1),
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("komari %s: HTTP %d: %s", method, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var rpcResp v2.Response
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return fmt.Errorf("komari %s: invalid response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("komari %s: %s", method, rpcResp.Error.Message)
	}
	return nil
}

func (c *KomariClient) Report(ctx context.Context, snapshot Snapshot) error {
	if !c.enabled() {
		return nil
	}
	if c.reportOnce {
		return c.send(ctx, v2.MethodAgentReport, v2.ReportParams{Report: snapshot.ToReport()})
	}
	// Several rigs often share one Komari instance; retry transient network
	// errors briefly so a restart or blip does not leave a gap in the history.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		if err = c.send(ctx, v2.MethodAgentReport, v2.ReportParams{Report: snapshot.ToReport()}); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
	}
	return err
}

func (c *KomariClient) UploadBasicInfo(ctx context.Context, snapshot Snapshot) error {
	if !c.enabled() {
		return nil
	}
	s := snapshot.System
	info := map[string]any{
		"cpu_name":           s.CPUName,
		"cpu_cores":          s.CPUCores,
		"cpu_physical_cores": s.CPUCores,
		"arch":               s.Arch,
		"os":                 snapshot.OSName(),
		"kernel_version":     s.Kernel,
		"mem_total":          s.RAMTotal,
		"swap_total":         s.SwapTotal,
		"disk_total":         s.DiskTotal,
		"gpu_name":           snapshot.GPUNameSummary(),
		"version":            "komari-miner-probe/" + Version,
	}
	return c.send(ctx, v2.MethodAgentBasicInfo, v2.BasicInfoParams{Info: info})
}
