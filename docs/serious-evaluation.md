# Serious Solari evaluation

`cmd/solari-probe` is a repeatable REST harness for evaluating Solari sandbox lifecycle behavior. It uses the documented Solari Sandbox API base URL and bearer-token authentication.

## Prerequisites

- A Solari console API key, format `slr_live_<id>_<secret>`.
- A Go toolchain.
- Network access to `https://api.getsolari.com`.

## Run a safe smoke test

Delete any unwanted test VMs from the Solari console first. Then start with one fresh and one reuse run per workload:

```sh
export SOLARI_TOKEN="slr_live_<id>_<secret>"
go run ./cmd/solari-probe -runs 1 -reuse-runs 1 -output results
```

The base URL defaults to `https://api.getsolari.com`; use `-base-url` only for a compatible custom gateway. The command writes `results/report.json`, `results/samples.csv`, `results/report.md`, and `results/latency.svg`. Preserve JSON/CSV when sharing conclusions, since aggregates hide retries, outliers, and failures.

## Verified API mapping

The harness uses the Solari Sandbox API contract:

| Operation | HTTP request | Expected response field |
|---|---|---|
| Create | `POST /sandboxes` | `sandboxId` |
| Execute | `POST /sandboxes/{sandboxId}/exec` | `exitCode`, `stdout`, `stderr` |
| Delete | `DELETE /sandboxes/{sandboxId}` | successful empty or JSON response |

Execution sends JSON similar to:

```json
{
  "cmd": "sh",
  "args": ["-c", "true"],
  "timeoutMs": 120000
}
```

The sandbox capability identifier is path-escaped before requests are made. The probe refuses to execute or delete if Solari does not return a `sandboxId`.

## Measurement design

The workloads are `noop` (`true`), a shell CPU loop, and an 8 MiB local write. Every fresh sample independently creates, executes, then deletes a sandbox:

```text
T_lifecycle = T_create + T_exec + T_delete
```

Reuse mode creates one sandbox per workload and measures repeated `/exec` calls against that same sandbox. Compare distributions rather than a one-off timing: P50 is typical latency; P95 is a practical tail-latency signal.

## Quota behavior

A `429 ConcurrencyLimitExceeded` result is recorded and stops the evaluation immediately. It is not retried, because another create would not resolve a concurrency-capacity limit. Delete obsolete VMs and begin with `-runs 1 -reuse-runs 1`; only increase repetition after the smoke test completes and cleanup is confirmed.

## Scope

This v1 harness covers create, exec, delete, and same-sandbox reuse. Snapshot, pause/resume, fork, network policy, and persistence-across-restart probes remain deferred until those API contracts are separately validated.
