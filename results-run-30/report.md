# Solari probe report

Generated: 2026-09-02T16:05:35Z

Endpoint: `https://api.getsolari.com`  
Fresh runs: 30; reuse runs: 30

## Latency summaries

| Series | n | Mean ms | P50 ms | P95 ms | Min ms | Max ms |
|---|---:|---:|---:|---:|---:|---:|
| cpu/fresh/create | 30 | 200.38 | 195.34 | 220.60 | 190.29 | 220.73 |
| cpu/fresh/delete | 30 | 177.31 | 176.60 | 183.01 | 176.02 | 188.24 |
| cpu/fresh/exec | 30 | 478.42 | 477.77 | 497.31 | 466.67 | 504.01 |
| cpu/reuse/create | 1 | 209.49 | 209.49 | 209.49 | 209.49 | 209.49 |
| cpu/reuse/delete | 1 | 176.18 | 176.18 | 176.18 | 176.18 | 176.18 |
| cpu/reuse/exec | 30 | 475.74 | 469.21 | 499.47 | 461.40 | 614.40 |
| io/fresh/create | 30 | 195.93 | 193.21 | 208.98 | 190.26 | 210.57 |
| io/fresh/delete | 30 | 176.32 | 176.28 | 176.72 | 176.04 | 177.18 |
| io/fresh/exec | 30 | 185.37 | 184.88 | 187.89 | 183.25 | 188.31 |
| io/reuse/create | 1 | 194.09 | 194.09 | 194.09 | 194.09 | 194.09 |
| io/reuse/delete | 1 | 178.35 | 178.35 | 178.35 | 178.35 | 178.35 |
| io/reuse/exec | 30 | 201.69 | 193.63 | 208.83 | 187.90 | 436.39 |
| noop/fresh/create | 29 | 219.18 | 194.39 | 231.62 | 189.89 | 804.87 |
| noop/fresh/delete | 29 | 176.60 | 176.41 | 177.68 | 176.12 | 178.47 |
| noop/fresh/exec | 29 | 179.78 | 179.81 | 181.58 | 178.64 | 182.28 |
| noop/reuse/create | 1 | 193.96 | 193.96 | 193.96 | 193.96 | 193.96 |
| noop/reuse/delete | 1 | 176.37 | 176.37 | 176.37 | 176.37 | 176.37 |
| noop/reuse/exec | 30 | 180.04 | 179.95 | 181.11 | 179.40 | 181.78 |

## Failure taxonomy

- operation:create: 1

## Interpretation

Fresh lifecycle is create + execute + delete for independently created sandboxes. Reuse measures repeated execution after one create; compare its execute distribution with fresh execution to isolate lifecycle overhead. P95 uses the nearest-rank sample at the rounded 95th-percentile index.
