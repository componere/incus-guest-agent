//go:build linux

package linux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-guest-agent/internal/agent"
)

// helperTimeout is the upper bound for helper-process readiness and reaping.
const helperTimeout = 5 * time.Second

func TestProcessRunForwardsSIGTERMToProcessGroup(t *testing.T) {
	bin := buildHelper(t, processGroupHelper)
	dir := t.TempDir()
	parentMark := filepath.Join(dir, "parent")
	childMark := filepath.Join(dir, "child")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := newTestProcess(bin, dir)
	proc.TermWaiter = NewWaiter(helperTimeout)
	proc.KillWaiter = NewWaiter(time.Millisecond)

	done := runProcess(ctx, t, proc)
	require.NoError(t, waitForFile(parentMark, helperTimeout))
	require.NoError(t, waitForFile(childMark, helperTimeout))
	cancel()

	err := waitProcess(t, done, helperTimeout)
	require.ErrorIs(t, err, agent.ErrShutdown)
	assert.FileExists(t, filepath.Join(dir, "parent-term"))
	assert.FileExists(t, filepath.Join(dir, "child-term"))
}

func TestProcessRunAdoptsAndReapsOrphans(t *testing.T) {
	bin := buildHelper(t, orphanHelper)
	dir := t.TempDir()
	ready := filepath.Join(dir, "orphan-ready")
	exited := filepath.Join(dir, "orphan-exit")

	proc := newTestProcess(bin, dir)
	proc.TermWaiter = NewWaiter(helperTimeout)
	proc.KillWaiter = NewWaiter(time.Millisecond)

	err := waitProcess(t, runProcess(context.Background(), t, proc), helperTimeout)
	require.ErrorIs(t, err, agent.ErrUnexpectedExit)
	assert.FileExists(t, ready)
	assert.FileExists(t, exited)
}

func TestProcessRunEscalatesToSIGKILLAfterInjectedGrace(t *testing.T) {
	bin := buildHelper(t, ignoreTermHelper)
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := newTestProcess(bin, dir)
	proc.TermWaiter = NewWaiter(0)
	proc.KillWaiter = NewWaiter(helperTimeout)

	done := runProcess(ctx, t, proc)
	require.NoError(t, waitForFile(ready, helperTimeout))
	cancel()

	err := waitProcess(t, done, helperTimeout)
	require.ErrorIs(t, err, agent.ErrShutdown)
	assert.NoFileExists(t, filepath.Join(dir, "term-exit"))
}

// newTestProcess returns a [Process] pointed at a helper binary in dir.
func newTestProcess(binary string, dir string) *Process {
	return &Process{
		Binary: binary,
		Dir:    dir,
		Stdin:  nil,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// runProcess starts [Process.Run] in the background and returns its result channel.
func runProcess(ctx context.Context, t *testing.T, proc *Process) <-chan error {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		done <- proc.Run(ctx)
	}()

	return done
}

// waitProcess waits for a background [Process.Run] result or fails the test.
func waitProcess(t *testing.T, done <-chan error, timeout time.Duration) error {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatal("process supervision did not return before timeout")
		return nil
	}
}

// waitForFile returns nil once path exists or [os.ErrNotExist] after timeout.
func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}

	return os.ErrNotExist
}

// buildHelper compiles source into a temporary helper binary.
func buildHelper(t *testing.T, source string) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(src, []byte(source), 0o600))
	bin := filepath.Join(dir, "helper")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build helper: %s", out)

	return bin
}

// processGroupHelper starts a child in the same process group and records SIGTERM.
const processGroupHelper = `package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "child" {
		awaitTerm(dir, "child")
		return
	}
	child := exec.Command(os.Args[0], "child")
	child.Dir = dir
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(1)
	}
	awaitTerm(dir, "parent")
}

func awaitTerm(dir string, name string) {
	ready, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		os.Exit(1)
	}
	_ = ready.Close()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	mark, err := os.Create(filepath.Join(dir, name+"-term"))
	if err != nil {
		os.Exit(1)
	}
	_ = mark.Close()
}
`

// orphanHelper starts a grandchild that outlives the direct child.
const orphanHelper = `package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	readyPath := filepath.Join(dir, "orphan-ready")
	if len(os.Args) > 1 && os.Args[1] == "orphan" {
		parent := os.Getppid()
		ready, err := os.Create(readyPath)
		if err != nil {
			os.Exit(1)
		}
		_ = ready.Close()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if os.Getppid() != parent {
				mark, err := os.Create(filepath.Join(dir, "orphan-exit"))
				if err != nil {
					os.Exit(1)
				}
				_ = mark.Close()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		os.Exit(1)
	}
	orphan := exec.Command(os.Args[0], "orphan")
	orphan.Dir = dir
	orphan.Stdout = os.Stdout
	orphan.Stderr = os.Stderr
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := orphan.Start(); err != nil {
		os.Exit(1)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(readyPath); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	os.Exit(1)
}
`

// ignoreTermHelper ignores SIGTERM so supervision must escalate to SIGKILL.
const ignoreTermHelper = `package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	signal.Ignore(syscall.SIGTERM)
	ready, err := os.Create(filepath.Join(dir, "ready"))
	if err != nil {
		os.Exit(1)
	}
	_ = ready.Close()
	for {
		time.Sleep(time.Hour)
	}
}
`
