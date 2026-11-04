package probe

import "testing"

func TestParseMinerSummaryRPC(t *testing.T) {
	body := []byte(`{
	  "id": 1, "jsonrpc": "2.0",
	  "result": {
	    "hashrate": {"total": [12345.6, 12000, 11000], "highest": 13000},
	    "results": {"shares_good": 10, "shares_total": 12},
	    "connection": {"pool": "pool.example:3333"},
	    "algo": "rx/0"
	  }
	}`)
	sum, err := parseMinerSummary(body, true)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Hashrate != 12345.6 || sum.SharesGood != 10 || sum.SharesTotal != 12 {
		t.Fatalf("unexpected summary: %#v", sum)
	}
	if sum.Pool != "pool.example:3333" || sum.Algorithm != "rx/0" {
		t.Fatalf("unexpected metadata: %#v", sum)
	}
}

func TestParseMinerSummaryDirect(t *testing.T) {
	body := []byte(`{"hashrate": 42.5, "results": {"shares_good": 3, "shares_rejected": 1}}`)
	sum, err := parseMinerSummary(body, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Hashrate != 42.5 || sum.SharesGood != 3 || sum.SharesTotal != 4 {
		t.Fatalf("unexpected summary: %#v", sum)
	}
}

func TestFormatHashrate(t *testing.T) {
	cases := map[float64]string{
		0:           "0 H/s",
		950:         "950 H/s",
		12_400:      "12.4 kH/s",
		62_400_000:  "62.4 MH/s",
		310_000_000: "310 MH/s",
	}
	for in, want := range cases {
		if got := FormatHashrate(in); got != want {
			t.Errorf("FormatHashrate(%v) = %q, want %q", in, got, want)
		}
	}
}
