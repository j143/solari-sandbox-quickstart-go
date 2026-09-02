# Serious Solari evaluation

`cmd/solari-probe` is a repeatable REST harness for evaluating sandbox lifecycle behavior. It is deliberately small enough to adapt while retaining raw measurements and reproducible aggregates.

## Prerequisites

- A Solari-compatible REST endpoint implementing `POST /sandboxes`, `POST /sandboxes/{id}/execute`, and `DELETE /sandboxes/{id}`.
- A Solari API key (bearer token).
- A Go toolchain.

## Run

```sh
go run ./cmd/solari-probe \
  -token "$SOLARI_API_KEY" \
  -runs 20 \
  -reuse-runs 10 \
  -output results
```

The base URL defaults to `https://api.solari.dev`. If your endpoint differs, override it with `-base-url "https://your-endpoint"`.

The command creates `results/report.json`, `results/samples.csv`, `results/report.md`, and `results/latency.svg`. Keep the JSON and CSV artifacts when sharing conclusions: summary statistics alone hide retries, outliers, and failure modes.

## What it measures

Three workloads are included: `noop` (`true`), a shell CPU loop, and an 8 MiB local write. Each fresh run measures create, execute, and delete on a newly created sandbox. Reuse mode creates one sandbox per workload, then records multiple executions against that same ID.

For a fresh sample, lifecycle time is:

```text
T_lifecycle = T_create + T_execute + T_delete
```

Reuse execution samples estimate steady-state command execution without repeatedly paying creation and teardown overhead. Compare distributions rather than one-off timings: P50 describes a typical run; P95 is a practical tail-latency signal. The implementation uses a rounded nearest-rank index after sorting measurements.

## API adaptation

The request and response structs in `cmd/solari-probe/types.go` intentionally isolate the assumed contract. If your endpoint uses another path, payload field, or response shape, update `Client.Create`, `Client.Execute`, `Client.Delete`, and the associated structs; do not change the measurement loop.

## Reading failures

Every failed operation is retained as a raw sample and counted by operation in the report. Inspect its `error` text before labeling a performance result: HTTP authorization, quota, provisioning, command exit, timeout, and response-decode failures represent different product behavior.

## Scope

This v1 harness evaluates create/execute/delete and same-sandbox reuse. Snapshot, pause/resume, fork, network policy, and persistence-across-restart probes are intentionally deferred until the deployed API contract for those lifecycle controls is validated.
