package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// version is injected when the runnable image is built.
var version = "dev"

// main runs the extension wrapper and returns its process status to the container runtime.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run handles the minimal command-line contract and starts the service loop.
func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v":
			_, _ = fmt.Fprintf(stdout, "incus-guest-agent %s\n", version)

			return 0
		case "--help", "-h":
			_, _ = fmt.Fprintln(stdout, "Usage: incus-guest-agent [--help] [--version]")

			return 0
		default:
			_, _ = fmt.Fprintf(stderr, "incus-guest-agent: unknown argument %q\n", args[0])

			return 2
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runAgent(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "incus-guest-agent: %v\n", err)

		return 1
	}

	return 0
}
