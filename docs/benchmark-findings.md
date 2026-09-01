# Solari Sandbox Benchmark Findings

**Experiment date:** 2026-09-02  
**Client environment:** GitHub Codespaces  
**Sandbox template:** `base`  
**Provisioning request:** 1 vCPU, 512 MiB  
**Benchmark source:** `benchmark_deep.go`

## Purpose

This experiment treats Solari as an agent-execution platform rather than only a remote command runner. The benchmark separates sandbox lifecycle cost from repeated execution cost, and checks two correctness properties that matter in an agent workflow:

- Does a guest-process failure preserve its exit code and standard error?
- Does state written in one command remain visible to a later command issued to the same sandbox?

The test uses one sandbox for all repeated commands. It therefore measures the lifecycle once, then samples the steady-state execution path without creating a VM for every request.

## Measured Results

| Measurement | Result | Meaning |
|---|---:|---|
| Sandbox create | 831.86 ms | `POST /sandboxes` until the API returned a runnable sandbox |
| First exec | 179.02 ms | First `POST /sandboxes/:id/exec` after creation |
| Create-to-first-result | 1010.88 ms | A practical time-to-first-output metric for a newly requested isolated environment |
| Steady-state P50 | 192.91 ms | Median latency of 30 sequential `exec` operations on the same sandbox |
| Steady-state P95 | 199.17 ms | 95th-percentile execution latency |
| Steady-state P99 | 200.19 ms | 99th-percentile execution latency |
| Guest error propagation | `true` | Exit code 23 and stderr text were returned correctly |
| File persistence | `false` | The write/read check did not observe the marker on a later REST exec request; this is unresolved, not yet a product-bug conclusion |

## Request Flow

```text
Client (GitHub Codespaces)
        |
        | POST /sandboxes
        | 831.86 ms observed
        v
Solari control plane
        |
        | returns a live sandbox ID
        v
One running sandbox
        |
        | POST /sandboxes/:id/exec
        | first execution: 179.02 ms
        |
        | POST /sandboxes/:id/exec x 30
        | steady-state: P50 192.91 ms, P95 199.17 ms, P99 200.19 ms
        v
stdout / stderr / exitCode returned to client
        |
        | DELETE /sandboxes/:id
        v
Sandbox cleanup
```

## Latency Model

The benchmark records a user-relevant lifecycle metric:

\[
T_{\text{create-to-first-result}}
=
T_{\text{POST /sandboxes}}
+
T_{\text{first POST /sandboxes/:id/exec}}
\]

For this run:

\[
T_{\text{create-to-first-result}}
=
831.86\ \text{ms}
+
179.02\ \text{ms}
=
1010.88\ \text{ms}
\]

The repeated-command path avoids creating another sandbox. Compared with the steady-state median, the approximate lifecycle cost avoided by reusing the live sandbox is:

\[
\Delta_{\text{reuse}}
=
T_{\text{create-to-first-result}}
-
T_{\text{steady-state,P50}}
\]

\[
\Delta_{\text{reuse}}
=
1010.88\ \text{ms}
-
192.91\ \text{ms}
=
817.97\ \text{ms}
\]

This does **not** mean every later command saves precisely 817.97 ms. It is a comparison between one complete fresh-environment path and the observed median of a repeated command on an already-running sandbox. It is useful for reasoning about multi-step agent workflows.

The observed steady-state tail spread was:

\[
P99 - P50
=
200.19\ \text{ms}
-
192.91\ \text{ms}
=
7.28\ \text{ms}
\]

A 7.28 ms P50-to-P99 gap over this 30-sample sequential test indicates a tight distribution in this environment. It is promising evidence of predictable command dispatch, but it is not an SLA and should not be generalized to different regions, concurrency levels, templates, or workloads.

## Interpretation

### Lifecycle versus execution

The full request path from no sandbox to first command output took about 1.01 seconds. Most of that observed time was in sandbox creation (831.86 ms), while the first one-shot command took 179.02 ms.

For agent workloads with several short steps, keeping one sandbox alive can be materially better than requesting a new isolated environment at every step. The benchmark exposes this distinction rather than combining it into one opaque latency number.

### Tail behavior

The P50, P95, and P99 values were close together:

```text
P50  192.91 ms
P95  199.17 ms
P99  200.19 ms
```

That is a useful early signal of stable sequential behavior. A future run should retain the raw sample file so the distribution can be graphed and compared across regions or releases.

### Guest failure semantics

The benchmark ran a guest command that emitted `benchmark-error` to stderr and exited with status 23. The result was correctly reported as a guest-command failure rather than a transport failure.

This distinction is important for agent orchestration:

```text
HTTP/API success: Solari successfully executed the request.
Guest success:    exitCode == 0.
Guest failure:    exitCode != 0; inspect stderr and decide whether to repair, retry, or stop.
```

## File Persistence: Unresolved Result

The benchmark wrote a unique marker to `/tmp/solari-benchmark-state.txt`, then issued a separate REST `exec` call to read it. The final boolean was `false`.

This result is intentionally documented as **unresolved**. It does not by itself prove that Solari sandboxes lack a persistent filesystem. Plausible explanations include:

1. The REST exec path uses a different working-directory, mount, or execution namespace policy than assumed by the test.
2. The selected `/tmp` path may not be the supported persistent location for this API path.
3. The write command or returned output requires more diagnostics.
4. The platform may intentionally isolate one-shot executions more aggressively than the sandbox resource abstraction suggests.

### Follow-up diagnostic

Before calling this a product issue, capture `exitCode`, `stdout`, and `stderr` for both the write and read operations, then inspect the execution environment:

```sh
pwd
id
ls -la /
ls -la /tmp
```

Run a write/read pair in the discovered writable directory and record both raw responses. A useful next variant also compares REST `exec` state to the documented control-WebSocket file API, if the exact protocol is available from an official SDK or reference.

## Methodology and Limits

- The benchmark creates **one** sandbox and reuses it for 30 sequential REST `exec` requests.
- It uses the documented REST lifecycle: create, exec, delete.
- The reported timings are client-observed end-to-end latencies. They include network transit, API/control-plane overhead, process startup, and guest execution; they do not isolate any one component perfectly.
- The run was performed from GitHub Codespaces. Results will vary by Codespaces region, Solari region, network path, account tier, template, VM size, and platform load.
- Thirty steady-state samples are enough for an exploratory signal, not for a production performance claim.
- No destructive resource-exhaustion test, high-concurrency test, or long timeout test was run.
- HTTP 429 should be treated as a concurrency-limit condition. It should trigger cleanup or capacity investigation rather than blind retries.

## Reproducing

Set an API key and run:

```bash
export SOLARI_API_KEY="..."
go run benchmark_deep.go
```

The program writes `sandbox-deep-results.json` and deletes the sandbox during normal process exit. If the process is interrupted forcefully, verify that the sandbox was deleted in the Solari console before rerunning; otherwise, an account concurrency limit may prevent a subsequent creation.

## Why No Graph Yet

A chart is only useful if it is based on the raw per-sample latency values. The current pasted run contains aggregate P50/P95/P99 values but does not include its generated JSON artifact in this repository. Adding a graph now would imply data that has not been committed.

The next enhancement should commit a sanitized `sandbox-deep-results.json` artifact or provide an analysis command that reads the artifact locally and generates a latency-distribution chart. Until then, the table, equations, and explicit methodology are more accurate than a decorative graph.
