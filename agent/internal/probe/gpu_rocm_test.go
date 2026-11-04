//go:build linux

package probe

import "testing"

func TestParseROCmOutput(t *testing.T) {
	// Shape produced by rocm-smi --json on ROCm 5.x/6.x (keys vary by version).
	out := `{
	  "card1": {
	    "Card series": "Radeon RX 7900 XTX",
	    "Card SKU": "D70201",
	    "VRAM Total Memory (B)": "25753026560",
	    "VRAM Total Used Memory (B)": "1024",
	    "GPU use (%)": "97",
	    "Temperature (Sensor edge) (C)": "61.0",
	    "Temperature (Sensor junction) (C)": "78.0",
	    "Current Power (W)": "312.5",
	    "Max Power (W)": "339.0",
	    "Fan speed (%)": "54",
	    "GPU Clock (MHz)": "2410"
	  },
	  "card0": {
	    "Card series": "Radeon RX 6800",
	    "VRAM Total Memory (B)": "17163091968",
	    "VRAM Total Used Memory (B)": "2048",
	    "GPU use (%)": "88",
	    "Temperature (Sensor edge) (C)": "58.0",
	    "Current Power (W)": "180",
	    "Fan speed (%)": "40"
	  }
	}`
	gpus, err := parseROCmOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs", len(gpus))
	}
	// Cards must be sorted by numeric index, not JSON key order.
	if gpus[0].Index != 0 || gpus[1].Index != 1 {
		t.Fatalf("indices = %d,%d", gpus[0].Index, gpus[1].Index)
	}
	if gpus[1].Name != "Radeon RX 7900 XTX" || gpus[1].Vendor != "AMD" {
		t.Fatalf("unexpected card 1: %#v", gpus[1])
	}
	if gpus[1].MemoryTotal != 25753026560 || gpus[1].MemoryUsed != 1024 {
		t.Fatalf("memory = %d/%d", gpus[1].MemoryUsed, gpus[1].MemoryTotal)
	}
	if gpus[1].Utilization != 97 || gpus[1].Temperature != 61 {
		t.Fatalf("util/temp = %v/%d", gpus[1].Utilization, gpus[1].Temperature)
	}
	if gpus[1].Power != 312.5 || gpus[1].PowerLimit != 339 || gpus[1].FanSpeed != 54 || gpus[1].ClockMHz != 2410 {
		t.Fatalf("power/fan/clock = %#v", gpus[1])
	}
}

func TestParseROCmOutputInvalid(t *testing.T) {
	if _, err := parseROCmOutput("not json"); err == nil {
		t.Fatal("expected parse error")
	}
}
