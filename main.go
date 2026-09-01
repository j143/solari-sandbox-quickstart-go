package main

import (
	"context"
	"fmt"
	"os"

	"github.com/solari-sdk/solari-sandbox-go"
)

// Sandbox quickstart — run untrusted code in a fresh microVM.
//
// A sandbox is a full Linux VM that boots from a memory snapshot, so it's
// usually ready in about a second. Nothing you run inside can touch your
// machine or another customer's.
//
// Usage:
//   export SOLARI_API_KEY="your-api-key"
//   go run main.go
func main() {
	apiKey := os.Getenv("SOLARI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: SOLARI_API_KEY environment variable is required")
		fmt.Fprintln(os.Stderr, "Get your API key at https://console.getsolari.com")
		os.Exit(1)
	}

	// Create a new Solari sandbox client
	client := solarisandbox.NewClient(apiKey)
	ctx := context.Background()

	// Run a simple Python script in an isolated microVM
	fmt.Println("Running Python code in a Solari sandbox...")
	resp, err := client.RunCode(ctx, &solarisandbox.RunCodeRequest{
		Code:     `print("Hello from Solari!")
print(f"2 + 2 = {2 + 2}")`,
		Language: "python",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running code: %v\n", err)
		os.Exit(1)
	}

	// Print the output
	if resp.Stdout != "" {
		fmt.Println("stdout:")
		fmt.Println(resp.Stdout)
	}
	if resp.Stderr != "" {
		fmt.Println("stderr:")
		fmt.Println(resp.Stderr)
	}

	fmt.Printf("Exit code: %d\n", resp.ExitCode)

	// Example: Run with file input
	fmt.Println("\nRunning Python code with file input...")
	resp2, err := client.RunCode(ctx, &solarisandbox.RunCodeRequest{
		Code: `
with open("data.txt", "r") as f:
    content = f.read()
print(f"File content: {content.strip()}")
print(f"Lines: {len(content.strip().splitlines())}")
`,
		Language: "python",
		Files: []solarisandbox.FileInput{
			{
				Path:    "data.txt",
				Content: "line1\nline2\nline3\n",
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running code with files: %v\n", err)
		os.Exit(1)
	}

	if resp2.Stdout != "" {
		fmt.Println("stdout:")
		fmt.Println(resp2.Stdout)
	}
	if resp2.Stderr != "" {
		fmt.Println("stderr:")
		fmt.Println(resp2.Stderr)
	}

	fmt.Printf("Exit code: %d\n", resp2.ExitCode)
}
