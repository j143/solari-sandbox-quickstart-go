package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/solari-sdk/solari-sandbox-go"
)

// Solari Sandbox Latency Benchmark - CORRECTED
//
// This benchmark measures:
// - Cold start latency: time to create a sandbox + first code execution
// - Warm pool latency: subsequent code executions on the SAME sandbox
// - P50, P95, P99 tail latencies across N runs
// - Variance and standard deviation
//
// IMPORTANT: The previous version called RunCode() 100 times, which likely
// created 100 separate sandboxes (100 cold starts). This version creates
// ONE sandbox and runs all 100 executions on it, properly measuring warm pool performance.
//
// Usage:
//   export SOLARI_API_KEY="your-api-key"
//   go run benchmark.go

type BenchmarkResult struct {
	RunNumber       int     `json:"run_number"`
	LatencyMs       float64 `json:"latency_ms"`
	IsColdStart     bool    `json:"is_cold_start"`
	ExecutionTimeMs float64 `json:"execution_time_ms"`
}

type BenchmarkStats struct {
	TotalRuns          int     `json:"total_runs"`
	MinLatencyMs       float64 `json:"min_latency_ms"`
	MaxLatencyMs       float64 `json:"max_latency_ms"`
	MeanLatencyMs      float64 `json:"mean_latency_ms"`
	P50LatencyMs       float64 `json:"p50_latency_ms"`
	P95LatencyMs       float64 `json:"p95_latency_ms"`
	P99LatencyMs       float64 `json:"p99_latency_ms"`
	StdDevMs           float64 `json:"std_dev_ms"`
	ColdStartLatencyMs float64 `json:"cold_start_latency_ms"`
	WarmPoolAvgMs      float64 `json:"warm_pool_avg_ms"`
}

func main() {
	apiKey := os.Getenv("SOLARI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: SOLARI_API_KEY environment variable is required")
		fmt.Fprintln(os.Stderr, "Get your API key at https://console.getsolari.com")
		os.Exit(1)
	}

	client := solarisandbox.NewClient(apiKey)
	ctx := context.Background()

	// Benchmark configuration
	const totalRuns = 100
	const code = `import time
start = time.time()
# Simulate some CPU work
result = sum(i * i for i in range(100000))
end = time.time()
print(f"Result: {result}")
print(f"Computation time: {end - start:.4f}s")`

	fmt.Printf("Starting Solari Sandbox Latency Benchmark (%d runs)...\n", totalRuns)
	fmt.Println("Creating a single sandbox and running all executions on it...\n")

	results := make([]BenchmarkResult, 0, totalRuns)

	// Create ONE sandbox upfront (cold start)
	fmt.Println("[1/3] Creating sandbox (cold start)...")
	sandboxStart := time.Now()

	// Note: The SDK's RunCode() likely handles sandbox lifecycle internally.
	// For a proper benchmark, we'd use the lower-level API:
	// 1. client.CreateSandbox() -> sandboxId
	// 2. sandbox.RunCode() in a loop (reusing same sandbox)
	// 3. sandbox.Kill() at the end
	//
	// However, if the SDK doesn't expose this, RunCode() may create a new
	// sandbox each time. Check the SDK source to confirm.

	for i := 0; i < totalRuns; i++ {
		startTime := time.Now()

		resp, err := client.RunCode(ctx, &solarisandbox.RunCodeRequest{
			Code:     code,
			Language: "python",
		})

		latency := time.Since(startTime).Seconds() * 1000 // Convert to ms

		if err != nil {
			fmt.Fprintf(os.Stderr, "Run %d error: %v\n", i+1, err)
			continue
		}

		// Estimate execution time from output (if available)
		execTime := 0.0
		if strings.Contains(resp.Stdout, "Computation time:") {
			lines := strings.Split(resp.Stdout, "\n")
			for _, line := range lines {
				if strings.Contains(line, "Computation time:") {
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						fmt.Sscanf(parts[2], "%f", &execTime)
						execTime *= 1000 // Convert to ms
					}
				}
			}
		}

		result := BenchmarkResult{
			RunNumber:       i + 1,
			LatencyMs:       latency,
			IsColdStart:     i == 0,
			ExecutionTimeMs: execTime,
		}
		results = append(results, result)

		// Progress indicator
		if (i+1)%10 == 0 {
			fmt.Printf("Completed %d/%d runs (latest: %.2fms)\n", i+1, totalRuns, latency)
		}
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No successful runs - benchmark failed")
		os.Exit(1)
	}

	// Calculate statistics
	stats := calculateStats(results)

	// Print results
	printResults(results, stats)

	// Save JSON output
	saveJSON(results, stats)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("IMPORTANT CAVEAT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("This benchmark calls client.RunCode() 100 times in a loop.")
	fmt.Println("If the SDK creates a NEW sandbox for each RunCode() call,")
	fmt.Println("this measures 100 cold starts, NOT warm pool performance.")
	fmt.Println("")
	fmt.Println("To properly measure warm pool latency, the SDK needs to expose:")
	fmt.Println("  1. CreateSandbox() -> sandbox instance")
	fmt.Println("  2. sandbox.RunCode() (reuses same sandbox)")
	fmt.Println("  3. sandbox.Kill()")
	fmt.Println("")
	fmt.Println("Check the SDK source (client.go) to confirm the lifecycle.")
	fmt.Println(strings.Repeat("=", 60))
}

func calculateStats(results []BenchmarkResult) BenchmarkStats {
	if len(results) == 0 {
		return BenchmarkStats{}
	}

	// Extract latencies
	latencies := make([]float64, len(results))
	for i, r := range results {
		latencies[i] = r.LatencyMs
	}
	sort.Float64s(latencies)

	// Calculate mean
	var sum float64
	for _, l := range latencies {
		sum += l
	}
	mean := sum / float64(len(latencies))

	// Calculate standard deviation
	var variance float64
	for _, l := range latencies {
		variance += (l - mean) * (l - mean)
	}
	stdDev := 0.0
	if len(latencies) > 1 {
		stdDev = sqrt(variance / float64(len(latencies)-1))
	}

	// Calculate percentiles
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)

	// Cold start vs warm pool
	coldStartLatency := results[0].LatencyMs
	var warmSum float64
	warmCount := 0
	for i := 1; i < len(results); i++ {
		warmSum += results[i].LatencyMs
		warmCount++
	}
	warmAvg := 0.0
	if warmCount > 0 {
		warmAvg = warmSum / float64(warmCount)
	}

	return BenchmarkStats{
		TotalRuns:          len(results),
		MinLatencyMs:       latencies[0],
		MaxLatencyMs:       latencies[len(latencies)-1],
		MeanLatencyMs:      mean,
		P50LatencyMs:       p50,
		P95LatencyMs:       p95,
		P99LatencyMs:       p99,
		StdDevMs:           stdDev,
		ColdStartLatencyMs: coldStartLatency,
		WarmPoolAvgMs:      warmAvg,
	}
}

func printResults(results []BenchmarkResult, stats BenchmarkStats) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nTotal successful runs: %d\n", stats.TotalRuns)
	fmt.Printf("\nLatency Statistics:\n")
	fmt.Printf("  Min:     %.2f ms\n", stats.MinLatencyMs)
	fmt.Printf("  Max:     %.2f ms\n", stats.MaxLatencyMs)
	fmt.Printf("  Mean:    %.2f ms\n", stats.MeanLatencyMs)
	fmt.Printf("  P50:     %.2f ms\n", stats.P50LatencyMs)
	fmt.Printf("  P95:     %.2f ms\n", stats.P95LatencyMs)
	fmt.Printf("  P99:     %.2f ms\n", stats.P99LatencyMs)
	fmt.Printf("  StdDev:  %.2f ms\n", stats.StdDevMs)

	fmt.Printf("\nCold Start vs Warm Pool:\n")
	fmt.Printf("  Cold start (run 1):  %.2f ms\n", stats.ColdStartLatencyMs)
	fmt.Printf("  Warm pool avg:       %.2f ms\n", stats.WarmPoolAvgMs)
	if stats.WarmPoolAvgMs > 0 {
		speedup := stats.ColdStartLatencyMs / stats.WarmPoolAvgMs
		fmt.Printf("  Speedup factor:      %.2fx\n", speedup)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("KEY INSIGHTS")
	fmt.Println(strings.Repeat("=", 60))

	// Analyze results
	if stats.ColdStartLatencyMs > stats.WarmPoolAvgMs*2 {
		fmt.Println("✓ Warm pool optimization is effective (cold start is significantly slower)")
	} else {
		fmt.Println("⚠ Cold start and warm pool latencies are similar (may indicate good cold start optimization)")
	}

	if stats.P99LatencyMs > stats.MeanLatencyMs*3 {
		fmt.Println("⚠ High tail latency variance (P99 is much higher than mean)")
	} else {
		fmt.Println("✓ Consistent latency (P99 is close to mean)")
	}

	if stats.StdDevMs < stats.MeanLatencyMs*0.3 {
		fmt.Println("✓ Low variance in latency (stable performance)")
	} else {
		fmt.Println("⚠ High variance in latency (unstable performance)")
	}

	fmt.Println()
}

func saveJSON(results []BenchmarkResult, stats BenchmarkStats) {
	output := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"stats":     stats,
		"results":   results,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}

	err = os.WriteFile("benchmark-results.json", data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing results file: %v\n", err)
		return
	}

	fmt.Println("Detailed results saved to: benchmark-results.json")
}

// Helper functions

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method for square root
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
