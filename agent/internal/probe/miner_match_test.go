package probe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchMinerName(t *testing.T) {
	cases := map[string]bool{
		"xmrig":             true,
		"XMRig":             true,
		"/opt/miners/t-rex": true,
		"lolMiner":          true,
		"nginx":             false,
		"komari-monitor":    false,
		"python3":           false,
	}
	for name, want := range cases {
		if got := matchMinerName(name, nil); got != want {
			t.Errorf("matchMinerName(%q) = %v, want %v", name, got, want)
		}
	}
	if !matchMinerName("custom-miner", []string{"custom-miner"}) {
		t.Error("extra pattern should match")
	}
}

func TestParseMinerArgs(t *testing.T) {
	args := []string{
		"/opt/t-rex", "-a", "kawpow",
		"-o", "stratum+tcp://pool.example.com:3333",
		"-u", "0xdeadbeef",
		"--http-port", "18080",
	}
	m := &Miner{}
	parseMinerArgs(m, args)
	if m.Algorithm != "kawpow" {
		t.Errorf("algorithm = %q", m.Algorithm)
	}
	if m.Pool != "stratum+tcp://pool.example.com:3333" {
		t.Errorf("pool = %q", m.Pool)
	}
	if m.Wallet != "0xdeadbeef" {
		t.Errorf("wallet = %q", m.Wallet)
	}
	if m.APIPort != 18080 {
		t.Errorf("api port = %d", m.APIPort)
	}

	eq := &Miner{}
	parseMinerArgs(eq, []string{"xmrig", "--url=stratum+tcp://x:1", "--user=wallet", "--algo=rx/0"})
	if eq.Pool != "stratum+tcp://x:1" || eq.Wallet != "wallet" || eq.Algorithm != "rx/0" {
		t.Errorf("equals form not parsed: %#v", eq)
	}
}

func TestBuildMinerDetectsUnknownBinaryWithStratumArgs(t *testing.T) {
	gpus := []GPU{{Index: 0, Processes: []GPUProcess{{PID: 42, Name: "mystery"}}}, {Index: 1}}
	candidate := processCandidate{
		PID:     42,
		Exe:     "/usr/local/bin/mystery-miner",
		Args:    []string{"mystery-miner", "--algo", "etchash", "-o", "stratum+tcp://pool:9999", "-u", "wallet"},
		Command: "mystery-miner --algo etchash -o stratum+tcp://pool:9999 -u wallet",
	}
	m, ok := buildMiner(candidate, gpus, nil)
	if !ok {
		t.Fatal("expected miner to be detected")
	}
	if len(m.GPUs) != 1 || m.GPUs[0] != 0 {
		t.Fatalf("gpu mapping = %v, want [0]", m.GPUs)
	}
	if m.Algorithm != "etchash" || m.Pool == "" {
		t.Fatalf("metadata not parsed: %#v", m)
	}

	if _, ok := buildMiner(processCandidate{PID: 7, Exe: "nginx", Args: []string{"nginx", "-g", "daemon off;"}}, gpus, nil); ok {
		t.Fatal("nginx must not be detected as a miner")
	}
}

func TestEnrichFromXMRigConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
	  "http": {"enabled": true, "port": 18081},
	  "pools": [{"url": "stratum+tcp://pool.example:3333", "user": "wallet-1", "algo": "rx/0"}]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Miner{}
	enrichFromXMRigConfig(m, []string{"xmrig", "-c", path})
	if m.APIPort != 18081 || m.Pool != "stratum+tcp://pool.example:3333" || m.Wallet != "wallet-1" || m.Algorithm != "rx/0" {
		t.Fatalf("unexpected config enrichment: %#v", m)
	}
}
