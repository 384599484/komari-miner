package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func testSnapshot() Snapshot {
	return Snapshot{
		Time:   time.Now(),
		Name:   "rig-1",
		System: System{CPUUsage: 12.5, RAMTotal: 100, RAMUsed: 40, DiskTotal: 200, DiskUsed: 80, Uptime: 1000, Processes: 42, TCP: 3, UDP: 4},
		GPUs: []GPU{{
			Index: 0, Name: "RTX 4090", MemoryTotal: 24 * mib, MemoryUsed: 12 * mib,
			Utilization: 91, Temperature: 70, Power: 300, PowerLimit: 450, FanSpeed: 55,
		}},
		Miners: []Miner{{
			PID: 42, Name: "t-rex", Algorithm: "kawpow", Hashrate: 62e6,
			Pool: "stratum+tcp://pool:3333", Wallet: "0xabc", GPUs: []int{0}, AcceptedShares: 10,
		}},
		TotalHashrate: 62e6,
		AvgGPUUsage:   91,
		TotalPower:    300,
		Alerts:        []string{"test alert"},
	}
}

func TestSnapshotToReport(t *testing.T) {
	rep := testSnapshot().ToReport()
	if rep.CPU.Usage != 12.5 || rep.Ram.Used != 40 {
		t.Fatalf("system mapping failed: %#v", rep)
	}
	if rep.GPU == nil || rep.GPU.Count != 1 || rep.GPU.DetailedInfo[0].Power != 300 {
		t.Fatalf("gpu mapping failed: %#v", rep.GPU)
	}
	if rep.Mining == nil || rep.Mining.MinerCount != 1 || rep.Mining.TotalHashrate != 62e6 {
		t.Fatalf("mining mapping failed: %#v", rep.Mining)
	}
	if rep.Mining.Miners[0].Hashrate != 62e6 || rep.Mining.Miners[0].GPUs[0] != 0 {
		t.Fatalf("miner mapping failed: %#v", rep.Mining.Miners[0])
	}
	if rep.Message != "test alert" {
		t.Fatalf("message = %q", rep.Message)
	}
}

func TestKomariClientReport(t *testing.T) {
	var gotPath, gotToken string
	var gotReq v2.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("token")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"success"}}`))
	}))
	defer ts.Close()

	client := NewKomariClient(ts.URL+"/", "secret-token")
	if err := client.Report(context.Background(), testSnapshot()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/clients/v2/rpc" || gotToken != "secret-token" {
		t.Fatalf("path/token = %q/%q", gotPath, gotToken)
	}
	if gotReq.Method != v2.MethodAgentReport {
		t.Fatalf("method = %q", gotReq.Method)
	}
	raw, _ := json.Marshal(gotReq.Params)
	var params v2.ReportParams
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	if params.Report.Mining == nil || params.Report.Mining.Miners[0].Name != "t-rex" {
		t.Fatalf("report payload missing mining: %s", raw)
	}
	if params.Report.GPU == nil || params.Report.GPU.DetailedInfo[0].FanSpeed != 55 {
		t.Fatalf("report payload missing gpu detail: %s", raw)
	}
}

func TestKomariClientBasicInfo(t *testing.T) {
	var info map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req v2.Request
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		raw, _ := json.Marshal(req.Params)
		var params v2.BasicInfoParams
		_ = json.Unmarshal(raw, &params)
		info = params.Info
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"success"}}`))
	}))
	defer ts.Close()

	client := NewKomariClient(ts.URL, "tok")
	if err := client.UploadBasicInfo(context.Background(), testSnapshot()); err != nil {
		t.Fatal(err)
	}
	if info["gpu_name"] != "1x RTX 4090" {
		t.Fatalf("gpu_name = %v", info["gpu_name"])
	}
	if !strings.Contains(info["version"].(string), "komari-miner-probe") {
		t.Fatalf("version = %v", info["version"])
	}
}

func TestKomariClientDisabledIsNoop(t *testing.T) {
	client := NewKomariClient("", "")
	if err := client.Report(context.Background(), testSnapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestKomariClientErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`))
	}))
	defer ts.Close()
	client := NewKomariClient(ts.URL, "tok")
	client.reportOnce = true
	err := client.Report(context.Background(), testSnapshot())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error, got %v", err)
	}
}
