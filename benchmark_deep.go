package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	apiBaseURL = "https://api.getsolari.com"
	sampleRuns = 30
	maxRetries = 3
)

type sandboxCreateRequest struct {
	Kind     string `json:"kind"`
	Template string `json:"template"`
	CPU      int    `json:"cpu"`
	MemMB    int    `json:"memMb"`
}

type sandboxCreateResponse struct {
	SandboxID  string `json:"sandboxId"`
	ControlURL string `json:"controlUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

type rpcRequest struct {
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type rpcResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  interface{}     `json:"error"`
}

type commandResult struct {
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
	TimestampUTC       string          `json:"timestamp_utc"`
	SandboxID          string          `json:"sandbox_id"`
	ExpiresAt          string          `json:"expires_at"`
	CreateLatencyMS    float64         `json:"create_latency_ms"`
	ConnectLatencyMS   float64         `json:"connect_latency_ms"`
	FirstCommandMS     float64         `json:"first_command_ms"`
	SteadyState        latencySummary  `json:"steady_state"`
	FilePersistenceOK  bool            `json:"file_persistence_ok"`
	ErrorPropagationOK bool            `json:"error_propagation_ok"`
	Notes              []string        `json:"notes"`
}

func main() {
	apiKey := os.Getenv("SOLARI_API_KEY")
	if apiKey == "" {
		fatalf("SOLARI_API_KEY is required")
	}

	ctx := context.Background()
	fmt.Println("Solari sandbox deep-dive benchmark")
	fmt.Println("Measures lifecycle, steady-state latency, state persistence, and error propagation.")

	createStarted := time.Now()
	sandbox, err := createSandboxWithRetry(ctx, apiKey)
	if err != nil {
		fatalf("create sandbox: %v", err)
	}
	createLatency := elapsedMS(createStarted)
	defer func() {
		if err := deleteSandbox(ctx, apiKey, sandbox.SandboxID); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		}
	}()
	fmt.Printf("sandbox created in %.2f ms\n", createLatency)

	connectStarted := time.Now()
	ws, _, err := websocket.DefaultDialer.Dial(sandbox.ControlURL, http.Header{
		"Authorization": []string{"Bearer " + apiKey},
	})
	if err != nil {
		fatalf("connect control channel: %v", err)
	}
	defer ws.Close()
	connectLatency := elapsedMS(connectStarted)
	fmt.Printf("control channel connected in %.2f ms\n", connectLatency)

	firstStarted := time.Now()
	first, err := runCode(ws, "printf first-run")
	if err != nil {
		fatalf("first command: %v", err)
	}
	if first.ExitCode != 0 || !strings.Contains(first.Stdout, "first-run") {
		fatalf("unexpected first command result: %+v", first)
	}
	firstLatency := elapsedMS(firstStarted)
	fmt.Printf("first command completed in %.2f ms\n", firstLatency)

	latencies := make([]float64, 0, sampleRuns)
	for i := 0; i < sampleRuns; i++ {
		started := time.Now()
		result, err := runCode(ws, "python3 -c 'print(sum(i*i for i in range(10000)))'")
		latency := elapsedMS(started)
		if err != nil {
			fatalf("sample %d: %v", i+1, err)
		}
		if result.ExitCode != 0 {
			fatalf("sample %d exited %d: %s", i+1, result.ExitCode, result.Stderr)
		}
		latencies = append(latencies, latency)
	}
	summary := summarise(latencies)
	fmt.Printf("steady state: p50 %.2f ms, p95 %.2f ms, p99 %.2f ms\n", summary.P50MS, summary.P95MS, summary.P99MS)

	marker := fmt.Sprintf("solari-state-%d", time.Now().UnixNano())
	_, err = runCode(ws, fmt.Sprintf("printf '%s' > /tmp/solari-benchmark-state.txt", marker))
	if err != nil {
		fatalf("write persistence marker: %v", err)
	}
	persisted, err := runCode(ws, "cat /tmp/solari-benchmark-state.txt")
	if err != nil {
		fatalf("read persistence marker: %v", err)
	}
	filePersistenceOK := persisted.ExitCode == 0 && strings.TrimSpace(persisted.Stdout) == marker

	failed, err := runCode(ws, "sh -c 'echo benchmark-error >&2; exit 23'")
	if err != nil {
		fatalf("failure scenario transport error: %v", err)
	}
	errorPropagationOK := failed.ExitCode == 23 && strings.Contains(failed.Stderr, "benchmark-error")

	report := deepReport{
		TimestampUTC:       time.Now().UTC().Format(time.RFC3339),
		SandboxID:          sandbox.SandboxID,
		ExpiresAt:          sandbox.ExpiresAt,
		CreateLatencyMS:    createLatency,
		ConnectLatencyMS:   connectLatency,
		FirstCommandMS:     firstLatency,
		SteadyState:        summary,
		FilePersistenceOK:  filePersistenceOK,
		ErrorPropagationOK: errorPropagationOK,
		Notes: []string{
			"Create latency includes the POST /sandboxes request until a usable control URL is returned.",
			"Steady-state samples reuse one sandbox and one WebSocket control channel.",
			"This measures client-observed end-to-end latency, including network transit and control-plane overhead.",
			"The test intentionally avoids destructive resource-exhaustion and long-running timeout scenarios.",
		},
	}
	writeReport(report)

	fmt.Printf("file persistence: %t\n", filePersistenceOK)
	fmt.Printf("error propagation: %t\n", errorPropagationOK)
	fmt.Println("detailed report saved to sandbox-deep-results.json")
}

func createSandboxWithRetry(ctx context.Context, apiKey string) (*sandboxCreateResponse, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		sandbox, err := createSandbox(ctx, apiKey)
		if err == nil {
			return sandbox, nil
		}
		lastErr = err
		
		// Check if it's a rate limit (429)
		if strings.Contains(err.Error(), "HTTP 429") {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 1s, 2s, 4s
			fmt.Printf("Rate limited, retrying in %v...\n", backoff)
			time.Sleep(backoff)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("max retries exceeded: %v", lastErr)
}

func createSandbox(ctx context.Context, apiKey string) (*sandboxCreateResponse, error) {
	body, err := json.Marshal(sandboxCreateRequest{Kind: "sandbox", Template: "base", CPU: 1, MemMB: 512})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/sandboxes", strings.NewReader(string(body)))
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
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create returned HTTP %d", resp.StatusCode)
	}
	var sandbox sandboxCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&sandbox); err != nil {
		return nil, err
	}
	if sandbox.SandboxID == "" || sandbox.ControlURL == "" {
		return nil, fmt.Errorf("create response missing sandboxId or controlUrl")
	}
	return &sandbox, nil
}

func deleteSandbox(ctx context.Context, apiKey, sandboxID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiBaseURL+"/sandboxes/"+sandboxID, nil)
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

func runCode(ws *websocket.Conn, code string) (*commandResult, error) {
	id := fmt.Sprintf("bench-%d", time.Now().UnixNano())
	request := rpcRequest{
		ID:     id,
		Method: "code.run",
		Params: map[string]interface{}{
			"code":     code,
			"language": "python",
			"timeout":  30,
		},
	}
	if err := ws.WriteJSON(request); err != nil {
		return nil, err
	}
	for {
		var response rpcResponse
		if err := ws.ReadJSON(&response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			var errMsg string
			switch e := response.Error.(type) {
			case string:
				errMsg = e
			case map[string]interface{}:
				if msg, ok := e["message"].(string); ok {
					errMsg = msg
				} else {
					errMsg = fmt.Sprintf("%v", e)
				}
			default:
				errMsg = fmt.Sprintf("%v", e)
			}
			return nil, fmt.Errorf("rpc error: %s", errMsg)
		}
		var result commandResult
		if err := json.Unmarshal(response.Result, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
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
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
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
