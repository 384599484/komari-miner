package probe

import (
	"strings"
	"testing"
)

// TestParseNvidiaRowsSkipsShortRecords guards against a stale header row or a
// truncated line shifting columns into the wrong fields.
func TestParseNvidiaRowsSkipsShortRecords(t *testing.T) {
	out := "0, GPU-a, RTX 4090, 24564, 1024, 91, 64, 70, 380\n" +
		"1, GPU-b, RTX 4090\n" +
		"2, GPU-c, RTX 4090, 24564, 1024, 95, 66, 72, 410\n"
	gpus, err := parseNvidiaRows(out, nvidiaFullFields)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 3 {
		t.Fatalf("got %d GPUs, want 3", len(gpus))
	}
	short := gpus[1]
	if short.Index != 1 || short.Name != "RTX 4090" || short.MemoryTotal != 0 || short.Temperature != 0 {
		t.Fatalf("short row should keep base fields only: %#v", short)
	}
	if gpus[0].Power != 380 || gpus[2].Power != 410 {
		t.Fatalf("full rows parsed incorrectly: %#v %#v", gpus[0], gpus[2])
	}
}

func TestBuildMinerNoGPUs(t *testing.T) {
	m, ok := buildMiner(processCandidate{
		PID: 9, Exe: "/usr/bin/xmrig", Args: []string{"xmrig", "--url", "pool:3333", "-u", "wallet"},
	}, nil, nil)
	if !ok {
		t.Fatal("xmrig should be detected")
	}
	if m.GPUs != nil {
		t.Fatalf("single-GPU fallback must not invent a GPU: %v", m.GPUs)
	}
}

func TestGPUIndexesForPID(t *testing.T) {
	gpus := []GPU{
		{Index: 0, Processes: []GPUProcess{{PID: 1}, {PID: 2}}},
		{Index: 3, Processes: []GPUProcess{{PID: 2}}},
	}
	if got := gpuIndexesForPID(gpus, 2); len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Fatalf("gpu indexes = %v", got)
	}
	if got := gpuIndexesForPID(gpus, 99); got != nil {
		t.Fatalf("unknown pid = %v", got)
	}
}

func TestFormatHashrateLargeValues(t *testing.T) {
	if got := FormatHashrate(1.5e12); !strings.HasSuffix(got, "TH/s") {
		t.Fatalf("1.5e12 = %q", got)
	}
	if got := FormatHashrate(-5); got != "0 H/s" {
		t.Fatalf("negative = %q", got)
	}
}
