# Solari Sandbox Quickstart (Go)

A minimal example showing how to run untrusted code in an isolated Solari sandbox using the Go SDK.

## What is a Solari Sandbox?

A Solari sandbox is a full Linux microVM that boots from a memory snapshot in about a second. It provides:

- **Hardware isolation** — each sandbox runs in its own microVM with a separate kernel
- **Ephemeral execution** — nothing persists between runs unless you explicitly save files
- **Language support** — Python, Node.js, Go, Rust, and more

## Prerequisites

- Go 1.23 or later
- A Solari API key (get one at [console.getsolari.com](https://console.getsolari.com))

## Setup

1. Clone this example:

   ```bash
   git clone https://github.com/j143/solari-sandbox-quickstart-go
   cd solari-sandbox-quickstart-go
   ```

2. Set your API key:

   ```bash
   export SOLARI_API_KEY="your-api-key-here"
   ```

3. Install dependencies and run:

   ```bash
   go mod tidy
   go run main.go
   ```

## What This Example Does

1. Creates a Solari sandbox client with your API key
2. Runs a simple Python script that prints output
3. Runs a second Python script that reads from a file input
4. Prints stdout, stderr, and exit codes for both executions

## Deep Benchmark

`benchmark_deep.go` measures the sandbox lifecycle using Solari's documented REST flow: create one sandbox, run a first command, reuse that sandbox for sequential commands, then delete it.

```bash
export SOLARI_API_KEY="your-api-key-here"
go run benchmark_deep.go
```

It records:

- Sandbox creation latency
- First-command latency
- Create-to-first-result latency
- Steady-state P50, P95, and P99 command latency
- File-state behavior across REST exec calls
- Guest exit-code and stderr propagation

The lifecycle metric is:

\[
T_{\text{create-to-first-result}}
=
T_{\text{POST /sandboxes}}
+
T_{\text{first POST /sandboxes/:id/exec}}
\]

Read the full [benchmark findings](docs/benchmark-findings.md) for methodology, measured GitHub Codespaces results, interpretation, limitations, and the unresolved filesystem-persistence diagnostic.

## Example Output

```
Running Python code in a Solari sandbox...
stdout:
Hello from Solari!
2 + 2 = 4

Exit code: 0

Running Python code with file input...
stdout:
File content: line1
line2
line3

Lines: 3

Exit code: 0
```

## Next Steps

- Check out the [full Go SDK documentation](https://github.com/solari-sdk/solari-sandbox-go)
- Explore other examples in the [Solari Cookbook](https://github.com/solari-sdk/solari-cookbook)
- Try the [browser quickstart](../browser-quickstart-go) to automate web interactions
