package probe

import "testing"

func TestParseNvidiaRows(t *testing.T) {
	out := `0, GPU-aaa, NVIDIA GeForce RTX 4090, 24564, 1024, [N/A], 65, 72, 380.5, 450, 60, 2520, 10501
1, GPU-bbb, "NVIDIA GeForce RTX 3090, Rev. A", 24576, 512, 88, 58, [N/A], [N/A], 350, [N/A], 1900, 9751
`
	gpus, err := parseNvidiaRows(out, nvidiaFullFields)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs", len(gpus))
	}
	g := gpus[0]
	if g.Index != 0 || g.Name != "NVIDIA GeForce RTX 4090" {
		t.Fatalf("unexpected gpu: %#v", g)
	}
	if g.MemoryTotal != 24564*mib || g.MemoryUsed != 1024*mib {
		t.Fatalf("memory conversion failed: %#v", g)
	}
	if g.Utilization != 0 || g.Temperature != 65 {
		t.Fatalf("N/A handling failed: %#v", g)
	}
	if g.Power != 380.5 || g.PowerLimit != 450 || g.FanSpeed != 60 || g.ClockMHz != 2520 {
		t.Fatalf("optional fields failed: %#v", g)
	}
	if gpus[1].Name != "NVIDIA GeForce RTX 3090, Rev. A" {
		t.Fatalf("quoted name failed: %q", gpus[1].Name)
	}
}

func TestParseNvidiaBaseFields(t *testing.T) {
	out := "0, GPU-x, RTX 3080, 10240, 2048, 75, 60\n"
	gpus, err := parseNvidiaRows(out, nvidiaBaseFields)
	if err != nil || len(gpus) != 1 {
		t.Fatalf("base parse: %v %#v", err, gpus)
	}
	if gpus[0].Power != 0 || gpus[0].FanSpeed != 0 {
		t.Fatalf("expected zero optional fields: %#v", gpus[0])
	}
}

func TestParseFloat(t *testing.T) {
	cases := map[string]float64{
		"380.5 W":         380.5,
		"[N/A]":           0,
		"60 %":            60,
		"[Not Supported]": 0,
		"24564 MiB":       24564,
	}
	for in, want := range cases {
		if got := parseFloat(in); got != want {
			t.Errorf("parseFloat(%q) = %v, want %v", in, got, want)
		}
	}
}
