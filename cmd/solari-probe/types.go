package main

import "time"

type Config struct {
	BaseURL   string
	Token     string
	Runs      int
	ReuseRuns int
	Timeout   time.Duration
	OutputDir string
}

type Workload struct {
	Name    string
	Command string
}

// Sandbox is the create response from POST /sandboxes. sandboxId is a signed
// capability identifier and must be URL-escaped when inserted into a path.
type Sandbox struct {
	ID     string `json:"sandboxId"`
	Status string `json:"status"`
}

type ExecuteRequest struct {
	Cmd       string   `json:"cmd"`
	Args      []string `json:"args,omitempty"`
	TimeoutMS int      `json:"timeoutMs,omitempty"`
}

type ExecuteResponse struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type Sample struct {
	Workload   string  `json:"workload"`
	Mode       string  `json:"mode"`
	Run        int     `json:"run"`
	Operation  string  `json:"operation"`
	DurationMS float64 `json:"duration_ms"`
	SandboxID  string  `json:"sandbox_id,omitempty"`
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
}

type Summary struct {
	Count  int     `json:"count"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	MinMS  float64 `json:"min_ms"`
	MaxMS  float64 `json:"max_ms"`
}

type Report struct {
	GeneratedAt time.Time          `json:"generated_at"`
	BaseURL     string             `json:"base_url"`
	FreshRuns   int                `json:"fresh_runs"`
	ReuseRuns   int                `json:"reuse_runs"`
	Samples     []Sample           `json:"samples"`
	Summaries   map[string]Summary `json:"summaries"`
	Failures    map[string]int     `json:"failures"`
}
