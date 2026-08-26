package agent

import (
	"context"
	"errors"
)

// DevicePath is a host path to one optical-device candidate.
type DevicePath string

var (
	// ErrInvalidMedia reports that a candidate was mounted but did not contain a
	// complete Incus guest-agent payload.
	ErrInvalidMedia = errors.New("invalid Incus guest-agent media")
	// ErrShutdown reports that the service or supervised process stopped because
	// shutdown was requested.
	ErrShutdown = errors.New("shutdown requested")
	// ErrUnexpectedExit reports that the staged incus-agent process tree ended
	// without a requested shutdown.
	ErrUnexpectedExit = errors.New("incus-agent exited unexpectedly")
)

// DeviceFinder returns lexically ordered optical-device candidates.
type DeviceFinder interface {
	// Find lists current device candidates without mounting them.
	Find(ctx context.Context) ([]DevicePath, error)
}

// StageManager mounts one candidate, stages the required files, and cleans
// mounts created by a failed or finished attempt.
type StageManager interface {
	// Stage mounts and validates device, then copies the five required files
	// into the staging area. It returns [ErrInvalidMedia] when the candidate is
	// not usable and any other error when preparation fails after a mount or
	// copy has been attempted.
	Stage(ctx context.Context, device DevicePath) error
	// Cleanup unmounts runtime mounts and removes incomplete staging output.
	Cleanup(ctx context.Context) error
}

// AgentProcess runs the staged incus-agent until exit or requested shutdown.
//
//nolint:revive // AgentProcess is the architecture port name.
type AgentProcess interface {
	// Run supervises the staged process tree until it exits or ctx is canceled.
	Run(ctx context.Context) error
}

// Waiter blocks for the fixed poll interval or until cancellation.
type Waiter interface {
	// Wait returns nil after the poll interval and a non-nil error when ctx is
	// canceled first.
	Wait(ctx context.Context) error
}
