package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/componere/incus-guest-agent/internal/agent"
)

const (
	// exitUnknownArg is returned when the command receives an unrecognized argument.
	exitUnknownArg = 2
)

//nolint:gochecknoglobals // GoReleaser injects these values with ldflags during releases.
var (
	// version is the release version injected at link time.
	version = "dev"
	// commit is the source commit injected at link time.
	commit = "none"
	// date is the build timestamp injected at link time.
	date = "unknown"
)

// main runs the guest-agent wrapper and returns its process status.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run handles the minimal command-line contract and starts the service loop.
func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 {
		return handleArgs(args, stdout, stderr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runService(ctx)
	if err == nil || errors.Is(err, agent.ErrShutdown) {
		return 0
	}
	if _, writeErr := fmt.Fprintf(stderr, "incus-guest-agent: %v\n", err); writeErr != nil {
		return 1
	}

	return 1
}

// handleArgs implements --help/--version and rejects every other argument.
func handleArgs(args []string, stdout io.Writer, stderr io.Writer) int {
	switch args[0] {
	case "--version", "-v":
		if _, err := fmt.Fprintln(stdout, versionText()); err != nil {
			return 1
		}

		return 0
	case "--help", "-h":
		if _, err := fmt.Fprintln(stdout, "Usage: incus-guest-agent [--help] [--version]"); err != nil {
			return 1
		}

		return 0
	default:
		if _, err := fmt.Fprintf(stderr, "incus-guest-agent: unknown argument %q\n", args[0]); err != nil {
			return 1
		}

		return exitUnknownArg
	}
}

// versionText returns the concise --version line and keeps linker-injected
// commit and date symbols referenced so -X stamps remain live.
func versionText() string {
	_, _ = commit, date

	return "incus-guest-agent " + version
}
