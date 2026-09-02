# Solari probe report

Generated: 2026-09-02T15:57:29Z

Endpoint: `https://api.getsolari.com`  
Fresh runs: 1; reuse runs: 1

## Latency summaries

| Series | n | Mean ms | P50 ms | P95 ms | Min ms | Max ms |
|---|---:|---:|---:|---:|---:|---:|
| cpu/fresh/create | 1 | 191.00 | 191.00 | 191.00 | 191.00 | 191.00 |
| cpu/fresh/delete | 1 | 175.89 | 175.89 | 175.89 | 175.89 | 175.89 |
| cpu/fresh/exec | 1 | 497.48 | 497.48 | 497.48 | 497.48 | 497.48 |
| cpu/reuse/create | 1 | 189.09 | 189.09 | 189.09 | 189.09 | 189.09 |
| cpu/reuse/delete | 1 | 174.36 | 174.36 | 174.36 | 174.36 | 174.36 |
| cpu/reuse/exec | 1 | 478.85 | 478.85 | 478.85 | 478.85 | 478.85 |
| io/fresh/create | 1 | 202.03 | 202.03 | 202.03 | 202.03 | 202.03 |
| io/fresh/delete | 1 | 174.36 | 174.36 | 174.36 | 174.36 | 174.36 |
| io/fresh/exec | 1 | 182.92 | 182.92 | 182.92 | 182.92 | 182.92 |
| io/reuse/create | 1 | 190.43 | 190.43 | 190.43 | 190.43 | 190.43 |
| io/reuse/delete | 1 | 174.12 | 174.12 | 174.12 | 174.12 | 174.12 |
| io/reuse/exec | 1 | 182.81 | 182.81 | 182.81 | 182.81 | 182.81 |
| noop/fresh/create | 1 | 37172.35 | 37172.35 | 37172.35 | 37172.35 | 37172.35 |
| noop/fresh/delete | 1 | 174.22 | 174.22 | 174.22 | 174.22 | 174.22 |
| noop/fresh/exec | 1 | 178.80 | 178.80 | 178.80 | 178.80 | 178.80 |
| noop/reuse/create | 1 | 203.06 | 203.06 | 203.06 | 203.06 | 203.06 |
| noop/reuse/delete | 1 | 174.24 | 174.24 | 174.24 | 174.24 | 174.24 |
| noop/reuse/exec | 1 | 177.16 | 177.16 | 177.16 | 177.16 | 177.16 |

## Failure taxonomy

No failures recorded.

## Interpretation

Fresh lifecycle is create + execute + delete for independently created sandboxes. Reuse measures repeated execution after one create; compare its execute distribution with fresh execution to isolate lifecycle overhead. P95 uses the nearest-rank sample at the rounded 95th-percentile index.
