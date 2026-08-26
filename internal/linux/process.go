//go:build linux

package linux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/componere/incus-guest-agent/internal/agent"
)

const (
	// prSetChildSubreaper is the prctl operation that enables child-subreaper mode.
	prSetChildSubreaper = 36
	// childWaitBuffer is the reaper event channel capacity.
	childWaitBuffer = 16
)

// Process supervises the staged incus-agent process tree.
type Process struct {
	// Binary is the staged executable path.
	Binary string
	// Dir is the working directory used for the staged agent.
	Dir string
	// Stdin is forwarded to the staged agent.
	Stdin io.Reader
	// Stdout receives unbuffered agent standard output.
	Stdout io.Writer
	// Stderr receives unbuffered agent standard error.
	Stderr io.Writer
	// TermWaiter waits after SIGTERM before SIGKILL is sent.
	TermWaiter agent.Waiter
	// KillWaiter waits after SIGKILL before supervision fails.
	KillWaiter agent.Waiter
}

// childWait reports one wait4 result from the process-tree reaper.
type childWait struct {
	// pid is the reaped child process ID.
	pid int
	// status is the child's kernel wait status.
	status syscall.WaitStatus
	// err is the wait4 error, including ECHILD when the process tree is empty.
	err error
}

// supervisor reaps the agent process group and applies the shutdown deadlines.
type supervisor struct {
	// ctx is canceled when the wrapper should stop the agent.
	ctx context.Context
	// pid is the direct child process ID and process-group ID.
	pid int
	// events receives wait4 results from the reaper.
	events <-chan childWait
	// term waits after SIGTERM.
	term agent.Waiter
	// kill waits after SIGKILL.
	kill agent.Waiter
}

// supervisorState tracks process-tree shutdown and reaping state.
type supervisorState struct {
	// direct is the direct child's exit status, if observed.
	direct *syscall.WaitStatus
	// requested reports whether context cancellation initiated shutdown.
	requested bool
	// killSent reports whether shutdown escalated to SIGKILL.
	killSent bool
	// grace emits when the active shutdown grace period ends.
	grace <-chan error
	// ctxDone emits when the caller requests shutdown.
	ctxDone <-chan struct{}
}

// NewProcess constructs a [Process] that runs the staged incus-agent binary.
func NewProcess() *Process {
	return &Process{
		Binary:     filepath.Join(StagePath(), agentBinaryName),
		Dir:        StagePath(),
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		TermWaiter: NewWaiter(ShutdownGrace),
		KillWaiter: NewWaiter(KillGrace),
	}
}

// Run starts the staged agent in its own process group and waits until the
// complete descendant tree exits or shutdown fails.
func (p *Process) Run(ctx context.Context) error {
	if err := becomeSubreaper(); err != nil {
		return fmt.Errorf("enable child subreaper: %w", err)
	}

	//nolint:gosec,noctx // Binary is explicit; CommandContext would bypass supervised group shutdown.
	command := exec.Command(p.Binary)
	command.Dir = p.Dir
	command.Stdin = p.Stdin
	command.Stdout = p.Stdout
	command.Stderr = p.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := command.Start(); err != nil {
		return fmt.Errorf("start incus-agent: %w", err)
	}

	events := make(chan childWait, childWaitBuffer)
	go reapChildren(events)

	return (&supervisor{
		ctx:    ctx,
		pid:    command.Process.Pid,
		events: events,
		term:   p.termWaiter(),
		kill:   p.killWaiter(),
	}).wait()
}

// termWaiter returns the SIGTERM grace waiter, substituting the default when unset.
func (p *Process) termWaiter() agent.Waiter {
	if p.TermWaiter != nil {
		return p.TermWaiter
	}

	return NewWaiter(ShutdownGrace)
}

// killWaiter returns the SIGKILL grace waiter, substituting the default when unset.
func (p *Process) killWaiter() agent.Waiter {
	if p.KillWaiter != nil {
		return p.KillWaiter
	}

	return NewWaiter(KillGrace)
}

// wait forwards shutdown signals and reaps every descendant.
func (s *supervisor) wait() error {
	state := supervisorState{ctxDone: s.ctx.Done()}

	for {
		select {
		case <-state.ctxDone:
			if err := s.requestShutdown(&state); err != nil {
				return err
			}
		case event := <-s.events:
			done, err := s.consumeWait(&state, event)
			if done {
				return err
			}
		case <-state.grace:
			if err := s.escalateShutdown(&state); err != nil {
				return err
			}
		}
	}
}

// requestShutdown begins graceful shutdown after context cancellation.
func (s *supervisor) requestShutdown(state *supervisorState) error {
	state.ctxDone = nil
	state.requested = true

	return s.startTermination(state)
}

// consumeWait applies one child-reaper result to the supervisor state.
func (s *supervisor) consumeWait(state *supervisorState, event childWait) (bool, error) {
	status, done, err := handleWait(event, s.pid, state.requested, state.direct)
	if status == nil {
		return done, err
	}

	state.direct = status
	if state.grace != nil {
		return done, err
	}
	if signalErr := s.startTermination(state); signalErr != nil {
		return true, signalErr
	}

	return done, err
}

// startTermination sends SIGTERM and starts the graceful shutdown deadline.
func (s *supervisor) startTermination(state *supervisorState) error {
	if err := signalProcessGroup(s.pid, syscall.SIGTERM); err != nil {
		return err
	}
	state.grace = startWait(s.term)

	return nil
}

// escalateShutdown sends SIGKILL or reports a process tree that survived it.
func (s *supervisor) escalateShutdown(state *supervisorState) error {
	if state.killSent {
		return fmt.Errorf("%w: process tree did not exit after SIGKILL", agent.ErrUnexpectedExit)
	}
	if err := signalProcessGroup(s.pid, syscall.SIGKILL); err != nil {
		return err
	}
	state.killSent = true
	state.grace = startWait(s.kill)

	return nil
}

// handleWait consumes one reaper event. A non-nil status is the direct child's
// exit status. done reports that the process tree is empty or wait failed.
func handleWait(
	event childWait,
	childPID int,
	requested bool,
	direct *syscall.WaitStatus,
) (*syscall.WaitStatus, bool, error) {
	if errors.Is(event.err, syscall.ECHILD) {
		return nil, true, treeExitError(requested, direct)
	}
	if event.err != nil {
		return nil, true, fmt.Errorf("wait for incus-agent process tree: %w", event.err)
	}
	if event.pid != childPID {
		return nil, false, nil
	}

	status := event.status

	return &status, false, nil
}

// treeExitError maps an empty process tree onto the public shutdown or
// unexpected-exit sentinels.
func treeExitError(requested bool, direct *syscall.WaitStatus) error {
	if requested {
		return agent.ErrShutdown
	}
	if direct == nil {
		return fmt.Errorf("%w: process tree disappeared without an exit status", agent.ErrUnexpectedExit)
	}

	return agentExitError(*direct)
}

// startWait begins a grace interval that is independent of the canceled
// service context so the SIGTERM and SIGKILL deadlines can elapse.
func startWait(wait agent.Waiter) <-chan error {
	events := make(chan error, 1)
	go func() {
		events <- wait.Wait(context.Background())
	}()

	return events
}

// reapChildren waits for every direct or reparented child and reports each result.
func reapChildren(events chan<- childWait) {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, 0, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		events <- childWait{pid: pid, status: status, err: err}
		if err != nil {
			return
		}
	}
}

// signalProcessGroup sends a shutdown signal to the agent's entire process group.
func signalProcessGroup(groupID int, signal syscall.Signal) error {
	err := syscall.Kill(-groupID, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}

	return fmt.Errorf("signal incus-agent process group with %s: %w", signal, err)
}

// becomeSubreaper makes the wrapper adopt and reap orphaned agent descendants
// when it is not PID 1.
func becomeSubreaper() error {
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}

	return nil
}

// agentExitError describes an unexpected exit from the host-supplied agent.
func agentExitError(status syscall.WaitStatus) error {
	if status.Exited() {
		return fmt.Errorf("%w: exited with status %d", agent.ErrUnexpectedExit, status.ExitStatus())
	}
	if status.Signaled() {
		return fmt.Errorf("%w: exited from signal %s", agent.ErrUnexpectedExit, status.Signal())
	}

	return fmt.Errorf("%w: wait status %d", agent.ErrUnexpectedExit, status)
}
