package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var workloads = []Workload{
	{Name: "noop", Command: "true"},
	{Name: "cpu", Command: "i=0; while [ $i -lt 250000 ]; do i=$((i+1)); done; echo cpu"},
	{Name: "io", Command: "dd if=/dev/zero of=/tmp/probe.bin bs=1M count=8 status=none && wc -c /tmp/probe.bin"},
}

func NewReport(cfg Config) Report {
	return Report{GeneratedAt: time.Now().UTC(), BaseURL: cfg.BaseURL, FreshRuns: cfg.Runs, ReuseRuns: cfg.ReuseRuns, Summaries: map[string]Summary{}, Failures: map[string]int{}}
}

func RunEvaluation(ctx context.Context, c *Client, r *Report) error {
	var errs []error
	for _, w := range workloads {
		for i := 1; i <= r.FreshRuns; i++ {
			if err := freshRun(ctx, c, r, w, i); err != nil {
				errs = append(errs, err)
				if IsConcurrencyLimit(err) {
					r.Summarize()
					return fmt.Errorf("stopped after Solari concurrency limit: %w", err)
				}
			}
		}
		if err := reuseRun(ctx, c, r, w); err != nil {
			errs = append(errs, err)
			if IsConcurrencyLimit(err) {
				r.Summarize()
				return fmt.Errorf("stopped after Solari concurrency limit: %w", err)
			}
		}
	}
	r.Summarize()
	return errors.Join(errs...)
}

func freshRun(ctx context.Context, c *Client, r *Report, w Workload, run int) error {
	s, err := measuredCreate(ctx, c, r, w.Name, "fresh", run)
	if err != nil {
		return err
	}
	defer measuredDelete(ctx, c, r, w.Name, "fresh", run, s.ID)
	return measuredExecute(ctx, c, r, w.Name, "fresh", run, s.ID, w.Command)
}

func reuseRun(ctx context.Context, c *Client, r *Report, w Workload) error {
	s, err := measuredCreate(ctx, c, r, w.Name, "reuse", 0)
	if err != nil {
		return err
	}
	defer measuredDelete(ctx, c, r, w.Name, "reuse", 0, s.ID)
	for i := 1; i <= r.ReuseRuns; i++ {
		if err := measuredExecute(ctx, c, r, w.Name, "reuse", i, s.ID, w.Command); err != nil {
			return err
		}
	}
	return nil
}

func measuredCreate(ctx context.Context, c *Client, r *Report, workload, mode string, run int) (Sandbox, error) {
	var s Sandbox
	ms, err := timed(func() error {
		var e error
		s, e = c.Create(ctx)
		return e
	})
	r.add(Sample{Workload: workload, Mode: mode, Run: run, Operation: "create", DurationMS: ms, SandboxID: s.ID, Status: status(err), Error: errText(err)})
	return s, err
}

func measuredExecute(ctx context.Context, c *Client, r *Report, workload, mode string, run int, id, command string) error {
	var response ExecuteResponse
	ms, err := timed(func() error {
		var e error
		response, e = c.Execute(ctx, id, command)
		return e
	})
	if err == nil && response.ExitCode != 0 {
		err = fmt.Errorf("exit code %d: %s", response.ExitCode, strings.TrimSpace(response.Stderr))
	}
	r.add(Sample{Workload: workload, Mode: mode, Run: run, Operation: "exec", DurationMS: ms, SandboxID: id, Status: status(err), Error: errText(err)})
	return err
}

func measuredDelete(ctx context.Context, c *Client, r *Report, workload, mode string, run int, id string) {
	ms, err := timed(func() error { return c.Delete(ctx, id) })
	r.add(Sample{Workload: workload, Mode: mode, Run: run, Operation: "delete", DurationMS: ms, SandboxID: id, Status: status(err), Error: errText(err)})
}

func (r *Report) add(s Sample) {
	r.Samples = append(r.Samples, s)
	if s.Status != "ok" {
		r.Failures["operation:"+s.Operation]++
	}
}

func (r *Report) Summarize() {
	groups := map[string][]float64{}
	for _, s := range r.Samples {
		if s.Status == "ok" {
			key := s.Workload + "/" + s.Mode + "/" + s.Operation
			groups[key] = append(groups[key], s.DurationMS)
		}
	}
	for key, values := range groups {
		r.Summaries[key] = summarize(values)
	}
}

func summarize(values []float64) Summary {
	sort.Float64s(values)
	var total float64
	for _, v := range values {
		total += v
	}
	return Summary{Count: len(values), MeanMS: total / float64(len(values)), P50MS: percentile(values, .50), P95MS: percentile(values, .95), MinMS: values[0], MaxMS: values[len(values)-1]}
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 1 {
		return values[0]
	}
	idx := int(float64(len(values)-1)*p + .5)
	return values[idx]
}

func status(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func errText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
