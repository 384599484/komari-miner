// Command komari-miner-probe is a lightweight mining-rig monitoring probe.
//
// It collects GPU telemetry (nvidia-smi / rocm-smi), detects mining processes,
// serves a self-contained dashboard and optionally reports to a Komari server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/384599484/komari-miner/agent/internal/probe"
)

type fileConfig struct {
	Name             string   `json:"name"`
	Listen           string   `json:"listen"`
	Interval         int      `json:"interval"`
	KomariURL        string   `json:"komari_url"`
	Token            string   `json:"token"`
	Demo             bool     `json:"demo"`
	DisableDashboard bool     `json:"disable_dashboard"`
	MinerPatterns    []string `json:"miner_patterns"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[miner-probe] ")

	cfg := probe.Config{
		Version:   probe.Version,
		Interval:  5 * time.Second,
		Listen:    ":9090",
		KomariURL: "",
		Token:     "",
	}

	configPath := prescanConfigPath(os.Args[1:])
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			log.Fatalf("read config %s: %v", configPath, err)
		}
		var fc fileConfig
		if err := json.Unmarshal(data, &fc); err != nil {
			log.Fatalf("parse config %s: %v", configPath, err)
		}
		applyFileConfig(&cfg, fc)
	}
	applyEnv(&cfg)

	fs := flag.NewFlagSet("komari-miner-probe", flag.ExitOnError)
	name := fs.String("name", cfg.Name, "探针/矿机显示名称 (env: PROBE_NAME)")
	config := fs.String("config", configPath, "JSON 配置文件路径")
	listen := fs.String("listen", cfg.Listen, "内置看板监听地址 (env: PROBE_LISTEN)")
	interval := fs.Int("interval", int(cfg.Interval.Seconds()), "采集与上报间隔秒数 (env: PROBE_INTERVAL)")
	komariURL := fs.String("komari", cfg.KomariURL, "Komari 服务端地址，例如 http://10.0.0.2:25774 (env: KOMARI_SERVER)")
	token := fs.String("token", cfg.Token, "Komari 客户端 token/UUID (env: KOMARI_TOKEN)")
	demo := fs.Bool("demo", cfg.Demo, "使用模拟矿机数据（无需 GPU）(env: PROBE_DEMO)")
	noDashboard := fs.Bool("no-dashboard", cfg.DisableDash, "关闭内置看板，仅上报")
	miners := fs.String("miners", strings.Join(cfg.MinerPatterns, ","), "额外的 miner 进程名匹配片段，逗号分隔")
	once := fs.Bool("once", false, "采集一次并以 JSON 输出后退出")
	showVersion := fs.Bool("version", false, "打印版本号")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "komari-miner-probe %s\n\n", probe.Version)
		fmt.Fprintf(os.Stderr, "用法: komari-miner-probe [flags]\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("komari-miner-probe %s\n", probe.Version)
		return
	}
	if *interval < 1 {
		*interval = 1
	}
	if *config != "" && *config != configPath {
		// The config flag may also be given after parsing; reload it so the
		// precedence stays: defaults < config file < env < flags.
		data, err := os.ReadFile(*config)
		if err != nil {
			log.Fatalf("read config %s: %v", *config, err)
		}
		var fc fileConfig
		if err := json.Unmarshal(data, &fc); err != nil {
			log.Fatalf("parse config %s: %v", *config, err)
		}
		applyFileConfig(&cfg, fc)
		applyEnv(&cfg)
	}
	cfg = probe.Config{
		Name:          strings.TrimSpace(*name),
		Listen:        strings.TrimSpace(*listen),
		Interval:      time.Duration(*interval) * time.Second,
		KomariURL:     strings.TrimSpace(*komariURL),
		Token:         strings.TrimSpace(*token),
		Demo:          *demo,
		DisableDash:   *noDashboard,
		MinerPatterns: splitPatterns(*miners),
		Version:       probe.Version,
	}

	collector := probe.NewCollector(cfg)
	if *once {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(collector.Collect()); err != nil {
			log.Fatalf("encode snapshot: %v", err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var dashboard *probe.Dashboard
	if !cfg.DisableDash && cfg.Listen != "" {
		dashboard = probe.NewDashboard(cfg)
		go func() {
			if err := dashboard.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("dashboard stopped: %v", err)
				stop()
			}
		}()
	}

	client := probe.NewKomariClient(cfg.KomariURL, cfg.Token)
	if cfg.KomariURL != "" && cfg.Token == "" {
		log.Printf("KOMARI 服务端已配置但缺少 token，跳过上报")
	}
	log.Printf("probe %s starting: rig=%q interval=%s demo=%v report=%v",
		probe.Version, cfg.Name, cfg.Interval, cfg.Demo, cfg.KomariURL != "" && cfg.Token != "")

	run(ctx, collector, client, cfg, dashboard)
	log.Printf("probe stopped")
}

func run(ctx context.Context, collector *probe.Collector, client *probe.KomariClient, cfg probe.Config, dashboard *probe.Dashboard) {
	report := func(s probe.Snapshot) {
		if client == nil {
			return
		}
		reportCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := client.Report(reportCtx, s); err != nil {
			log.Printf("report failed: %v", err)
		}
	}

	snapshot := collector.Collect()
	if dashboard != nil {
		dashboard.Publish(snapshot)
	}
	if client != nil {
		infoCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := client.UploadBasicInfo(infoCtx, snapshot); err != nil {
			log.Printf("basic info upload failed: %v", err)
		}
		cancel()
	}
	report(snapshot)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	infoTicker := time.NewTicker(30 * time.Minute)
	defer infoTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := collector.Collect()
			if dashboard != nil {
				dashboard.Publish(s)
			}
			report(s)
		case <-infoTicker.C:
			infoCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := client.UploadBasicInfo(infoCtx, collector.Collect()); err != nil {
				log.Printf("basic info upload failed: %v", err)
			}
			cancel()
		}
	}
}

func prescanConfigPath(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-config=") || strings.HasPrefix(arg, "--config=") {
			return strings.TrimSpace(strings.SplitN(arg, "=", 2)[1])
		}
		if (arg == "-config" || arg == "--config") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return strings.TrimSpace(args[i+1])
		}
	}
	return ""
}

func applyFileConfig(cfg *probe.Config, fc fileConfig) {
	if fc.Name != "" {
		cfg.Name = fc.Name
	}
	if fc.Listen != "" {
		cfg.Listen = fc.Listen
	}
	if fc.Interval > 0 {
		cfg.Interval = time.Duration(fc.Interval) * time.Second
	}
	if fc.KomariURL != "" {
		cfg.KomariURL = fc.KomariURL
	}
	if fc.Token != "" {
		cfg.Token = fc.Token
	}
	cfg.Demo = fc.Demo
	cfg.DisableDash = fc.DisableDashboard
	cfg.MinerPatterns = fc.MinerPatterns
}

func applyEnv(cfg *probe.Config) {
	if v := os.Getenv("PROBE_NAME"); v != "" {
		cfg.Name = v
	}
	if v := os.Getenv("PROBE_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("PROBE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		} else if secs, err := time.ParseDuration(v + "s"); err == nil {
			cfg.Interval = secs
		}
	}
	if v := os.Getenv("KOMARI_SERVER"); v != "" {
		cfg.KomariURL = v
	}
	if v := os.Getenv("KOMARI_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("PROBE_DEMO"); v != "" {
		cfg.Demo = parseBool(v)
	}
	if v := os.Getenv("PROBE_DISABLE_DASHBOARD"); v != "" {
		cfg.DisableDash = parseBool(v)
	}
	if v := os.Getenv("PROBE_MINER_PATTERNS"); v != "" {
		cfg.MinerPatterns = splitPatterns(v)
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y":
		return true
	}
	return false
}

func splitPatterns(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
