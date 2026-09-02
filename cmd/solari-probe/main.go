package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var cfg Config
	flag.StringVar(&cfg.BaseURL, "base-url", "https://api.getsolari.com", "Solari REST base URL (default: https://api.getsolari.com)")
	flag.StringVar(&cfg.Token, "token", os.Getenv("SOLARI_TOKEN"), "Bearer token (or SOLARI_TOKEN); format slr_live_<id>_<secret>")
	flag.IntVar(&cfg.Runs, "runs", 10, "fresh lifecycle runs per workload")
	flag.IntVar(&cfg.ReuseRuns, "reuse-runs", 5, "same-sandbox reuse runs per workload")
	flag.DurationVar(&cfg.Timeout, "timeout", 2*time.Minute, "per-request timeout")
	flag.StringVar(&cfg.OutputDir, "output", "results", "report output directory")
	flag.Parse()

	if cfg.Token == "" {
		log.Fatal("-token or SOLARI_TOKEN is required")
	}
	if cfg.Runs < 1 || cfg.ReuseRuns < 1 {
		log.Fatal("-runs and -reuse-runs must both be at least 1")
	}

	ctx := context.Background()
	client := NewClient(cfg)
	report := NewReport(cfg)
	if err := RunEvaluation(ctx, client, &report); err != nil {
		log.Printf("evaluation finished with failures: %v", err)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := writeJSON(filepath.Join(cfg.OutputDir, "report.json"), report); err != nil {
		log.Fatal(err)
	}
	if err := writeCSV(filepath.Join(cfg.OutputDir, "samples.csv"), report.Samples); err != nil {
		log.Fatal(err)
	}
	if err := writeMarkdown(filepath.Join(cfg.OutputDir, "report.md"), report); err != nil {
		log.Fatal(err)
	}
	if err := writeSVG(filepath.Join(cfg.OutputDir, "latency.svg"), report); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s/{report.json,samples.csv,report.md,latency.svg}\n", cfg.OutputDir)
}

func writeJSON(path string, value any) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeCSV(path string, samples []Sample) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"workload", "mode", "run", "operation", "duration_ms", "sandbox_id", "status", "error"}); err != nil { return err }
	for _, s := range samples {
		if err := w.Write([]string{s.Workload, s.Mode, fmt.Sprint(s.Run), s.Operation, fmt.Sprintf("%.3f", s.DurationMS), s.SandboxID, s.Status, s.Error}); err != nil { return err }
	}
	return w.Error()
}
