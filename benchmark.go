package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	apiBaseURL    = "https://api.getsolari.com"
	totalRuns     = 100
	benchmarkCode = `import time
start = time.time()
result = sum(i * i for i in range(100000))
end = time.time()
print(f"Result: {result}")
print(f"Computation time: {end - start:.4f}s")`
)

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

type CreateSandboxRequest struct {
	Kind     string `json:"kind,omitempty"`
	Template string `json:"template,omitempty"`
	CPU      int    `json:"cpu,omitempty"`
	MemMb    int    `json:"memMb,omitempty"`
}

type CreateSandboxResponse struct {
	SandboxID  string `json:"sandboxId"`
	Kind       string `json:"kind"`
	ControlURL string `json:"controlUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

type CodeRunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func main() {
	apiKey := os.Getenv("SOLARI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: SOLARI_API_KEY environment variable is required")
		fmt.Fprintln(os.Stderr, "Get your API key at https://console.getsolari.com")
		os.Exit(1)
	}

	ctx := context.Background()

	fmt.Printf("Starting Solari Sandbox Latency Benchmark (%d runs)\n", totalRuns)
	fmt.Println("Architecture: Create 1 sandbox → Run 100 code.runs over WebSocket → Destroy\n")

	fmt.Println("[1/4] Creating sandbox (cold start)...")
	sandboxStart := time.Now()

	sandbox, err := createSandbox(ctx, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create sandbox: %v\n", err)
		os.Exit(1)
	}

	coldStartTime := time.Since(sandboxStart).Seconds() * 1000
	fmt.Printf("Sandbox created: %s (took %.2fms)\n", sandbox.SandboxID, coldStartTime)

	fmt.Println("[2/4] Connecting to WebSocket control channel...")
	ws, _, err := websocket.DefaultDialer.Dial(sandbox.ControlURL, http.Header{
		"Authorization": []string{"Bearer " + apiKey},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect WebSocket: %v\n", err)
		deleteSandbox(ctx, apiKey, sandbox.SandboxID)
		os.Exit(1)
	}
	defer ws.Close()
	fmt.Println("WebSocket connected")

	fmt.Println("[3/4] Running benchmark (100 code executions)...")
	results := make([]BenchmarkResult, 0, totalRuns)

	for i := 0; i < totalRuns; i++ {
		startTime := time.Now()

		resp, err := runCode(ws, benchmarkCode, "python")
		latency := time.Since(startTime).Seconds() * 1000

		if err != nil {
			fmt.Fprintf(os.Stderr, "Run %d error: %v\n", i+1, err)
			continue
		}

		execTime := parseExecutionTime(resp.Stdout)

		result := BenchmarkResult{
			RunNumber:       i + 1,
			LatencyMs:       latency,
			IsColdStart:     i == 0,
			ExecutionTimeMs: execTime,
		}
		results = append(results, result)

		if (i+1)%10 == 0 {
			fmt.Printf("Completed %d/%d runs (latest: %.2fms)\n", i+1, totalRuns, latency)
		}
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No successful runs - benchmark failed")
		deleteSandbox(ctx, apiKey, sandbox.SandboxID)
		os.Exit(1)
	}

	fmt.Println("[4/4] Destroying sandbox...")
	if err := deleteSandbox(ctx, apiKey, sandbox.SandboxID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to delete sandbox: %v\n", err)
	}
	fmt.Println("Sandbox destroyed")

	stats := calculateStats(results)
	printResults(results, stats)
	saveJSON(results, stats)
}

func createSandbox(ctx context.Context, apiKey string) (*CreateSandboxResponse, error) {
	reqBody := CreateSandboxRequest{
		Kind:     "sandbox",
		Template: "base",
		CPU:      1,
		MemMb:    512,
	}

	req, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", apiBaseURL+"/sandboxes", strings.NewReader(string(req)))
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusCreated {
		var errBody map[string]interface{}
		json.NewDecoder(httpResp.Body).Decode(&errBody)
		return nil, fmt.Errorf("API returned status %d: %v", httpResp.StatusCode, errBody)
	}

	var resp CreateSandboxResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func runCode(ws *websocket.Conn, code, language string) (*CodeRunResponse, error) {
	err := ws.WriteJSON(map[string]interface{}{
		"type":     "code.run",
		"code":     code,
		"language": language,
		"timeout":  30,
	})
	if err != nil {
		return nil, err
	}

	var resp CodeRunResponse
	err = ws.ReadJSON(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func deleteSandbox(ctx context.Context, apiKey, sandboxID string) error {
	httpReq, _ := http.NewRequestWithContext(ctx, "DELETE", apiBaseURL+"/sandboxes/"+sandboxID, nil)
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

func parseExecutionTime(stdout string) float64 {
	if !strings.Contains(stdout, "Computation time:") {
		return 0
	}
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Computation time:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				var t float64
				if _, err := fmt.Sscanf(parts[2], "%f", &t); err == nil {
					return t * 1000
				}
			}
			break
		}
	}
	return 0
}

func calculateStats(results []BenchmarkResult) BenchmarkStats {
	if len(results) == 0 {
		return BenchmarkStats{}
	}

	latencies := make([]float64, len(results))
	for i, r := range results {
		latencies[i] = r.LatencyMs
	}
	sort.Float64s(latencies)

	var sum float64
	for _, l := range latencies {
		sum += l
	}
	mean := sum / float64(len(latencies))

	var variance float64
	for _, l := range latencies {
		variance += (l - mean) * (l - mean)
	}
	stdDev := 0.0
	if len(latencies) > 1 {
		stdDev = sqrt(variance / float64(len(latencies)-1))
	}

	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)

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

	if stats.ColdStartLatencyMs > stats.WarmPoolAvgMs*2 {
		fmt.Println("✓ Warm pool optimization is effective")
	} else {
		fmt.Println("⚠ Cold start and warm pool latencies are similar")
	}

	if stats.P99LatencyMs > stats.MeanLatencyMs*3 {
		fmt.Println("⚠ High tail latency variance")
	} else {
		fmt.Println("✓ Consistent latency")
	}

	if stats.StdDevMs < stats.MeanLatencyMs*0.3 {
		fmt.Println("✓ Low variance in latency")
	} else {
		fmt.Println("⚠ High variance in latency")
	}

	fmt.Println()
}

func saveJSON(results []BenchmarkResult, stats BenchmarkStats) {
	output := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"stats":     stats,
		"results":   results,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	os.WriteFile("benchmark-results.json", data, 0644)
	fmt.Println("Detailed results saved to: benchmark-results.json")
}

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
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
