package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var minerHTTP = &http.Client{Timeout: 1200 * time.Millisecond}

// fillMinerHashrates queries local miner APIs to obtain real hashrate and
// share counters. XMRig's JSON-RPC summary endpoint is supported out of the
// box; other miners simply keep zero values until an API is added.
func fillMinerHashrates(miners []Miner) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range miners {
		if miners[i].APIPort <= 0 {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			summary, err := queryXMRigSummary(miners[i].APIPort)
			if err != nil {
				return
			}
			if summary.Hashrate > 0 {
				miners[i].Hashrate = summary.Hashrate
			}
			if summary.SharesTotal > 0 {
				miners[i].AcceptedShares = summary.SharesGood
				miners[i].RejectedShares = summary.SharesTotal - summary.SharesGood
			}
			if summary.Pool != "" && miners[i].Pool == "" {
				miners[i].Pool = summary.Pool
			}
			if summary.Algorithm != "" && miners[i].Algorithm == "" {
				miners[i].Algorithm = summary.Algorithm
			}
		}(i)
	}
	wg.Wait()
}

type minerSummary struct {
	Hashrate    float64
	SharesGood  int64
	SharesTotal int64
	Pool        string
	Algorithm   string
}

func queryXMRigSummary(port int) (minerSummary, error) {
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	body := []byte(`{"id":1,"jsonrpc":"2.0","method":"summary"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/json_rpc", bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, err := minerHTTP.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				if sum, err := parseMinerSummary(data, true); err == nil && (sum.Hashrate > 0 || sum.SharesTotal > 0) {
					return sum, nil
				}
			}
		}
	}
	// Fallback used by several XMRig builds and proxies.
	for _, path := range []string{"/2/summary", "/summary"} {
		resp, err := minerHTTP.Get(base + path)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if sum, err := parseMinerSummary(data, false); err == nil && (sum.Hashrate > 0 || sum.SharesTotal > 0) {
				return sum, nil
			}
		}
	}
	return minerSummary{}, fmt.Errorf("miner api unavailable on port %d", port)
}

func parseMinerSummary(data []byte, rpc bool) (minerSummary, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return minerSummary{}, err
	}
	if rpc {
		if errObj, ok := root["error"]; ok && errObj != nil {
			return minerSummary{}, fmt.Errorf("miner api error")
		}
		if result, ok := root["result"].(map[string]any); ok {
			root = result
		}
	}
	sum := minerSummary{}
	if hashrate, ok := root["hashrate"]; ok {
		sum.Hashrate = firstNumber(hashrate)
	}
	if results, ok := root["results"].(map[string]any); ok {
		sum.SharesGood = int64(firstNumber(results["shares_good"]))
		sum.SharesTotal = int64(firstNumber(results["shares_total"]))
		if sum.SharesTotal == 0 {
			sum.SharesTotal = sum.SharesGood + int64(firstNumber(results["shares_rejected"]))
		}
	}
	if connection, ok := root["connection"].(map[string]any); ok {
		if pool, ok := connection["pool"].(string); ok {
			sum.Pool = strings.TrimSpace(pool)
		}
	}
	for _, key := range []string{"algo", "algorithm", "coin"} {
		if v, ok := root[key].(string); ok && v != "" {
			sum.Algorithm = v
			break
		}
	}
	if job, ok := root["job"].(map[string]any); ok && sum.Algorithm == "" {
		if v, ok := job["algorithm"].(string); ok {
			sum.Algorithm = v
		}
	}
	if sum.Hashrate < 0 {
		sum.Hashrate = 0
	}
	return sum, nil
}
