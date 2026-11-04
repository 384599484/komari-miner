package probe

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed assets/*
var assetsFS embed.FS

const historyLimit = 180

// Dashboard serves the probe's self-contained mining dashboard.
type Dashboard struct {
	cfg     Config
	mu      sync.RWMutex
	latest  Snapshot
	history []HistoryPoint
	started time.Time
	srv     *http.Server
}

func NewDashboard(cfg Config) *Dashboard {
	return &Dashboard{cfg: cfg, started: time.Now()}
}

// Publish stores the newest snapshot and appends one history sample.
func (d *Dashboard) Publish(s Snapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.latest = s
	d.history = append(d.history, s.History())
	if len(d.history) > historyLimit {
		d.history = append([]HistoryPoint{}, d.history[len(d.history)-historyLimit:]...)
	}
}

type statusResponse struct {
	Snapshot        *Snapshot      `json:"snapshot"`
	History         []HistoryPoint `json:"history"`
	Version         string         `json:"version"`
	ProbeUptime     int64          `json:"probe_uptime"`
	IntervalSeconds int            `json:"interval_seconds"`
}

func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assetsFS, "assets")
	fileServer := http.FileServer(http.FS(static))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		index, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "dashboard assets missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		_, _ = w.Write(index)
	})
	mux.HandleFunc("/api/status", d.handleStatus)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func (d *Dashboard) handleStatus(w http.ResponseWriter, _ *http.Request) {
	d.mu.RLock()
	snapshot := d.latest
	history := append([]HistoryPoint{}, d.history...)
	d.mu.RUnlock()
	resp := statusResponse{
		History:         history,
		Version:         Version,
		ProbeUptime:     int64(time.Since(d.started).Seconds()),
		IntervalSeconds: int(d.cfg.Interval.Seconds()),
	}
	if !snapshot.Time.IsZero() {
		resp.Snapshot = &snapshot
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	_ = enc.Encode(resp)
}

// Run binds the listen address and serves until ctx is cancelled. It blocks.
func (d *Dashboard) Run(ctx context.Context) error {
	addr := d.cfg.Listen
	if addr == "" {
		addr = ":9090"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("dashboard listen %s: %w", addr, err)
	}
	d.srv = &http.Server{
		Handler:           d.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = d.srv.Shutdown(shutdownCtx)
	}()
	log.Printf("dashboard listening on http://%s", ln.Addr().String())
	err = d.srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
