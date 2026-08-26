//go:build linux

package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/componere/incus-guest-agent/internal/agent"
	"github.com/componere/incus-guest-agent/internal/linux"
)

// runService wires the Linux adapters into the core supervisor.
func runService(ctx context.Context) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service := agent.New(
		linux.NewFinder(),
		linux.NewStager(),
		linux.NewProcess(),
		linux.NewWaiter(linux.PollInterval),
		logger,
	)

	return service.Run(ctx)
}
