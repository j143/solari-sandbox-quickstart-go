package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	apiBaseURL = "https://api.getsolari.com"
	sampleRuns = 30
)

type sandboxCreateRequest struct {
	Kind     string `json:"kind"`
	Template string `json:"template"`
	CPU      int    `json:"cpu"`
	MemMB    int    `json:"memMb"`
}

type sandboxCreateResponse struct {
	SandboxID string `json:"sandboxId"`
	ExpiresAt string `json:"expiresAt"`
}

type execRequest struct {
	Cmd       string   `json:"cmd"`
	Args      []string `json:"args,omitempty"`
	TimeoutMS int      `json:"timeoutMs"`
}

type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

type latencySummary struct {
	Count  int     `json:"count"`
	MinMS  float64 `json:"min_ms"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MaxMS  float64 `json:"max_ms"`
	StdMS  float64 `json:"stddev_ms"`
}

type deepReport struct {
	TimestampUTC          string         `json:"timestamp_utc"`
	SandboxID             string         `json:"sandbox_id"`
	ExpiresAt             string         `json:"expires_at"`
	CreateLatencyMS       float64        `json:"create_latency_ms"`
	FirstExecLatencyMS    float64        `json:"first_exec_latency_ms"`
	CreateToFirstResultMS float64        `json:"create_to_first_result_ms"`
	SteadyState           latencySummary `json:"steady_state"`
	FilePersistenceOK     bool           `json:"file_persistence_ok"`
	ErrorPropagationOK    bool           `json:"error_propagation_ok"`
	Notes                 []string       `json:"notes"`
}

func main() {
	apiKey := os.Getenv("SOLARI_API_KEY")
	if apiKey == "" {
		fatalf("SOLARI_API_KEY is required")
	}

	ctx := context.Background()
	fmt.Println("Solari sandbox deep-dive benchmark")
	fmt.Println("Uses the documented REST exec path; one sandbox is reused for all samples.")

	createStarted := time.Now()
	sandbox, err := createSandbox(ctx, apiKey)
	if err != nil {
		fatalf("create sandbox: %v", err)
	}
	createLatency := elapsedMS(createStarted)
	defer cleanup(ctx, apiKey, sandbox.SandboxID)
	fmt.Printf("sandbox created in %.2f ms\n", createLatency)

	firstStarted := time.Now()
	first, err := exec(ctx, apiKey, sandbox.SandboxID, "sh", []string{"-c", "printf first-run"})
	if err != nil {
		fatalf("first exec: %v", err)
	}
	firstLatency := elapsedMS(firstStarted)
	if first.ExitCode != 0 || strings.TrimSpace(first.Stdout) != "first-run" {
		fatalf("unexpected first exec result: exit=%d stdout=%q stderr=%q", first.ExitCode, first.Stdout, first.Stderr)
	}
	fmt.Printf("first exec completed in %.2f ms\n", firstLatency)
	fmt.Printf("create-to-first-result: %.2f ms\n", createLatency+firstLatency)

	latencies := make([]float64, 0, sampleRuns)
	for i := 0; i < sampleRuns; i++ {
		started := time.Now()
		result, err := exec(ctx, apiKey, sandbox.SandboxID, "python3", []string{"-c", "print(sum(i*i for i in range(10000)))"})
		latency := elapsedMS(started)
		if err != nil {
			fatalf("steady-state sample %d: %v", i+1, err)
		}
		if result.ExitCode != 0 {
			fatalf("steady-state sample %d exited %d: %s", i+1, result.ExitCode, result.Stderr)
		}
		latencies = append(latencies, latency)
	}
	steadyState := summarise(latencies)
	fmt.Printf("steady state (%d samples): p50 %.2f ms, p95 %.2f ms, p99 %.2f ms\n", sampleRuns, steadyState.P50MS, steadyState.P95MS, steadyState.P99MS)

	marker := fmt.Sprintf("solari-state-%d", time.Now().UnixNano())
	if _, err := exec(ctx, apiKey, sandbox.SandboxID, "sh", []string{"-c", "printf %s > /tmp/solari-benchmark-state.txt", marker}); err != nil {
		fatalf("write persistence marker: %v", err)
	}
	persisted, err := exec(ctx, apiKey, sandbox.SandboxID, "cat", []string{"/tmp/solari-benchmark-state.txt"})
	if err != nil {
		fatalf("read persistence marker: %v", err)
	}
	filePersistenceOK := persisted.ExitCode == 0 && strings.TrimSpace(persisted.Stdout) == marker

	failed, err := exec(ctx, apiKey, sandbox.SandboxID, "sh", []string{"-c", "echo benchmark-error >&2; exit 23"})
	if err != nil {
		fatalf("failure scenario transport error: %v", err)
	}
	errorPropagationOK := failed.ExitCode == 23 && strings.Contains(failed.Stderr, "benchmark-error")

	report := deepReport{
		TimestampUTC:          time.Now().UTC().Format(time.RFC3339),
		SandboxID:             sandbox.SandboxID,
		ExpiresAt:             sandbox.ExpiresAt,
		CreateLatencyMS:       createLatency,
		FirstExecLatencyMS:    firstLatency,
		CreateToFirstResultMS: createLatency + firstLatency,
		SteadyState:           steadyState,
		FilePersistenceOK:     filePersistenceOK,
		ErrorPropagationOK:    errorPropagationOK,
		Notes: []string{
			"Formula: T_create-to-first-result = T_POST /sandboxes + T_first POST /sandboxes/:id/exec.",
			"Steady-state samples reuse exactly one sandbox and issue sequential REST exec requests.",
			"Measurements are client-observed end-to-end timings and include network, control-plane, and guest execution overhead.",
			"HTTP 429 is treated as a concurrency-limit condition; the benchmark does not retry it blindly.",
			"The benchmark avoids destructive resource-exhaustion and long-running timeout scenarios.",
		},
	}
	writeReport(report)

	fmt.Printf("file persistence: %t\n", filePersistenceOK)
	fmt.Printf("error propagation: %t\n", errorPropagationOK)
	fmt.Println("detailed report saved to sandbox-deep-results.json")
}

func createSandbox(ctx context.Context, apiKey string) (*sandboxCreateResponse, error) {
	body, err := json.Marshal(sandboxCreateRequest{Kind: "sandbox", Template: "base", CPU: 1, MemMB: 512})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/sandboxes", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("create returned HTTP 429: concurrency limit reached; delete or wait for an existing sandbox to expire")
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create returned HTTP %d", resp.StatusCode)
	}
	var sandbox sandboxCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&sandbox); err != nil {
		return nil, err
	}
	if sandbox.SandboxID == "" {
		return nil, fmt.Errorf("create response missing sandboxId")
	}
	return &sandbox, nil
}

func exec(ctx context.Context, apiKey, sandboxID, cmd string, args []string) (*execResponse, error) {
	body, err := json.Marshal(execRequest{Cmd: cmd, Args: args, TimeoutMS: 30000})
	if err != nil {
		return nil, err
	}
	endpoint := apiBaseURL + "/sandboxes/" + url.PathEscape(sandboxID) + "/exec"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exec returned HTTP %d", resp.StatusCode)
	}
	var result execResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func cleanup(ctx context.Context, apiKey, sandboxID string) {
	if err := deleteSandbox(ctx, apiKey, sandboxID); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		return
	}
	fmt.Println("sandbox deleted")
}

func deleteSandbox(ctx context.Context, apiKey, sandboxID string) error {
	endpoint := apiBaseURL + "/sandboxes/" + url.PathEscape(sandboxID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func summarise(values []float64) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	var total float64
	for _, value := range sorted {
		total += value
	}
	mean := total / float64(len(sorted))
	var sumSquares float64
	for _, value := range sorted {
		delta := value - mean
		sumSquares += delta * delta
	}
	return latencySummary{
		Count:  len(sorted),
		MinMS:  sorted[0],
		MeanMS: mean,
		P50MS:  quantile(sorted, 0.50),
		P95MS:  quantile(sorted, 0.95),
		P99MS:  quantile(sorted, 0.99),
		MaxMS:  sorted[len(sorted)-1],
		StdMS:  math.Sqrt(sumSquares / float64(len(sorted))),
	}
}

func quantile(sorted []float64, q float64) float64 {
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		return sorted[0]
	}
	if index >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[index]
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}

func idempotencyKey() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("benchmark-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func writeReport(report deepReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatalf("serialize report: %v", err)
	}
	if err := os.WriteFile("sandbox-deep-results.json", data, 0644); err != nil {
		fatalf("write report: %v", err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
