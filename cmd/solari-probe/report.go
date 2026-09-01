package main

import (
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
)

func writeMarkdown(path string, r Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Solari probe report\n\nGenerated: %s\n\n", r.GeneratedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&b, "Endpoint: `%s`  \nFresh runs: %d; reuse runs: %d\n\n", r.BaseURL, r.FreshRuns, r.ReuseRuns)
	b.WriteString("## Latency summaries\n\n| Series | n | Mean ms | P50 ms | P95 ms | Min ms | Max ms |\n|---|---:|---:|---:|---:|---:|---:|\n")
	keys := sortedSummaryKeys(r.Summaries)
	for _, key := range keys { s := r.Summaries[key]; fmt.Fprintf(&b, "| %s | %d | %.2f | %.2f | %.2f | %.2f | %.2f |\n", key, s.Count, s.MeanMS, s.P50MS, s.P95MS, s.MinMS, s.MaxMS) }
	b.WriteString("\n## Failure taxonomy\n\n")
	if len(r.Failures) == 0 { b.WriteString("No failures recorded.\n") } else { for key, n := range r.Failures { fmt.Fprintf(&b, "- %s: %d\n", key, n) } }
	b.WriteString("\n## Interpretation\n\nFresh lifecycle is create + execute + delete for independently created sandboxes. Reuse measures repeated execution after one create; compare its execute distribution with fresh execution to isolate lifecycle overhead. P95 uses the nearest-rank sample at the rounded 95th-percentile index.\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func sortedSummaryKeys(m map[string]Summary) []string {
	keys := make([]string, 0, len(m))
	for k := range m { keys = append(keys, k) }
	sort.Strings(keys)
	return keys
}

func writeSVG(path string, r Report) error {
	keys := sortedSummaryKeys(r.Summaries)
	width, height := 1000, 80+len(keys)*42
	max := 1.0
	for _, s := range r.Summaries { if s.P95MS > max { max = s.P95MS } }
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\"><style>text{font:14px sans-serif}.label{font-size:12px}.bar{fill:#276ef1}</style><text x=\"20\" y=\"28\">Solari probe P95 latency (ms)</text>", width, height, width, height)
	for i, key := range keys {
		y := 58+i*42
		s := r.Summaries[key]
		w := int(700 * s.P95MS / max)
		fmt.Fprintf(&b, "<text class=\"label\" x=\"20\" y=\"%d\">%s</text><rect class=\"bar\" x=\"270\" y=\"%d\" width=\"%d\" height=\"20\"/><text x=\"%d\" y=\"%d\">%.1f</text>", y+15, html.EscapeString(key), y, w, 280+w, y+15, s.P95MS)
	}
	b.WriteString("</svg>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
