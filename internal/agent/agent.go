package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Service probes for Incus media, stages the first valid payload, and runs
// exactly one agent process.
type Service struct {
	// finder lists optical-device candidates.
	finder DeviceFinder
	// stage mounts, validates, and copies media.
	stage StageManager
	// proc supervises the staged agent.
	proc AgentProcess
	// wait delays between retryable probe attempts.
	wait Waiter
	// logger records state transitions and retryable failures.
	logger *slog.Logger
}

// New constructs a [Service] from concrete port implementations.
//
// A nil logger is replaced with a discarded logger. The remaining ports are
// required and must remain non-nil for the lifetime of the service.
func New(finder DeviceFinder, stage StageManager, proc AgentProcess, wait Waiter, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Service{
		finder: finder,
		stage:  stage,
		proc:   proc,
		wait:   wait,
		logger: logger,
	}
}

// Run probes for valid media until one payload is staged and one process is
// started, or until ctx is canceled.
//
// Missing devices, [ErrInvalidMedia], and preparation errors are logged and
// retried after [Waiter.Wait]. Context cancellation returns an error wrapping
// [ErrShutdown]. A successful stage starts exactly one [AgentProcess.Run].
func (s *Service) Run(ctx context.Context) error {
	s.logger.InfoContext(ctx, "waiting for Incus configuration media")

	for {
		if err := ctx.Err(); err != nil {
			return shutdownError(err)
		}

		if !s.probe(ctx) {
			if waitErr := s.wait.Wait(ctx); waitErr != nil {
				return shutdownError(waitErr)
			}

			continue
		}

		return s.runStaged(ctx)
	}
}

// probe reconciles leftover mounts, discovers candidates, and stages the first
// valid medium. A false result means the caller should wait and retry.
func (s *Service) probe(ctx context.Context) bool {
	if err := s.stage.Cleanup(ctx); err != nil {
		s.logger.ErrorContext(ctx, "failed to clean runtime state", slog.Any("err", err))

		return false
	}

	devices, err := s.finder.Find(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to discover optical devices", slog.Any("err", err))

		return false
	}

	return s.stageFirst(ctx, devices)
}

// stageFirst tries each candidate in order. Invalid media continues to the next
// device. Preparation errors clean leftover state and request a retry.
func (s *Service) stageFirst(ctx context.Context, devices []DevicePath) bool {
	for _, device := range devices {
		err := s.stage.Stage(ctx, device)
		if err == nil {
			s.logger.InfoContext(ctx, "staged Incus agent files", slog.String("device", string(device)))

			return true
		}
		if errors.Is(err, ErrInvalidMedia) {
			s.logger.InfoContext(ctx, "skipping invalid media", slog.String("device", string(device)))

			continue
		}

		s.logger.ErrorContext(
			ctx,
			"failed to stage Incus media",
			slog.String("device", string(device)),
			slog.Any("err", err),
		)
		if cleanupErr := s.stage.Cleanup(ctx); cleanupErr != nil {
			s.logger.ErrorContext(ctx, "failed to clean runtime state", slog.Any("err", cleanupErr))
		}

		return false
	}

	return false
}

// runStaged starts the staged agent once and always cleans runtime mounts after
// that process returns.
func (s *Service) runStaged(ctx context.Context) error {
	s.logger.InfoContext(ctx, "starting host-supplied incus-agent")

	runErr := s.proc.Run(ctx)
	if cleanupErr := s.stage.Cleanup(ctx); cleanupErr != nil {
		s.logger.ErrorContext(ctx, "failed to clean runtime state", slog.Any("err", cleanupErr))
		if runErr == nil {
			return cleanupErr
		}
	}

	return runErr
}

// shutdownError wraps a cancellation cause with [ErrShutdown].
func shutdownError(err error) error {
	if err == nil {
		return ErrShutdown
	}

	return fmt.Errorf("%w: %w", ErrShutdown, err)
}
