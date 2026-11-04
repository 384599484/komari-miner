package probe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboardStatus(t *testing.T) {
	d := NewDashboard(Config{Interval: 5 * time.Second, Demo: true})
	s := testSnapshot()
	d.Publish(s)
	d.Publish(s)
	ts := httptest.NewServer(d.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Snapshot == nil || len(body.Snapshot.Miners) != 1 {
		t.Fatalf("snapshot missing: %#v", body.Snapshot)
	}
	if len(body.History) != 2 {
		t.Fatalf("history = %d", len(body.History))
	}
	if body.IntervalSeconds != 5 {
		t.Fatalf("interval = %d", body.IntervalSeconds)
	}

	index, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer index.Body.Close()
	buf := make([]byte, 4096)
	n, _ := index.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "KOMARI") {
		t.Fatalf("index page missing brand: %q", string(buf[:n]))
	}
}
