//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

const (
	runtimeRoot         = "/var/run/incus-guest-agent"
	mediaMountPath      = runtimeRoot + "/media"
	stageMountPath      = runtimeRoot + "/agent"
	mediaPollInterval   = 2 * time.Second
	shutdownGrace       = 10 * time.Second
	killGrace           = 2 * time.Second
	prSetChildSubreaper = 36
)

// stagedFile describes one required file on the Incus configuration medium.
type stagedFile struct {
	// name is the source and destination basename.
	name string
	// executable reports whether the file must have an executable mode bit.
	executable bool
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

// requiredMediaFiles is the complete Incus guest-agent media contract.
var requiredMediaFiles = [...]stagedFile{
	{name: "agent.conf"},
	{name: "server.crt"},
	{name: "agent.crt"},
	{name: "agent.key"},
	{name: "incus-agent", executable: true},
}

// runAgent reconciles stale mounts, waits for Incus media, and supervises the host-supplied agent.
func runAgent(ctx context.Context) (err error) {
	if err := becomeSubreaper(); err != nil {
		return fmt.Errorf("enable child subreaper: %w", err)
	}

	if err := prepareRuntime(); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanupRuntime())
	}()

	log.Printf("waiting for Incus configuration media under /dev/sr*")

	for {
		staged, stageErr := discoverAndStage()
		if stageErr != nil {
			return stageErr
		}
		if staged {
			log.Printf("starting host-supplied incus-agent")

			return superviseAgent(ctx)
		}

		timer := time.NewTimer(mediaPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}

			return nil
		case <-timer.C:
		}
	}
}

// becomeSubreaper makes the wrapper adopt and reap orphaned agent descendants when it is not PID 1.
func becomeSubreaper() error {
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}

	return nil
}

// prepareRuntime removes mounts left by an abnormal prior exit and recreates empty mountpoints.
func prepareRuntime() error {
	if err := cleanupRuntime(); err != nil {
		return fmt.Errorf("reconcile stale runtime state: %w", err)
	}
	if err := os.MkdirAll(mediaMountPath, 0o700); err != nil {
		return fmt.Errorf("create media mountpoint: %w", err)
	}
	if err := os.MkdirAll(stageMountPath, 0o700); err != nil {
		return fmt.Errorf("create staging mountpoint: %w", err)
	}

	return nil
}

// cleanupRuntime lazily detaches runtime mounts and removes their private directory.
func cleanupRuntime() error {
	var cleanupErr error
	for _, path := range []string{stageMountPath, mediaMountPath} {
		if err := lazyUnmount(path); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("unmount %s: %w", path, err))
		}
	}
	if err := os.RemoveAll(runtimeRoot); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove %s: %w", runtimeRoot, err))
	}

	return cleanupErr
}

// lazyUnmount detaches a mount while accepting a missing or unmounted path.
func lazyUnmount(path string) error {
	err := syscall.Unmount(path, syscall.MNT_DETACH)
	if err == nil || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) {
		return nil
	}

	return err
}

// discoverAndStage probes optical block devices and stages the first valid Incus medium.
func discoverAndStage() (bool, error) {
	devices, err := filepath.Glob("/dev/sr*")
	if err != nil {
		return false, fmt.Errorf("enumerate optical devices: %w", err)
	}
	sort.Strings(devices)

	for _, device := range devices {
		matched, stageErr := stageDevice(device)
		if stageErr != nil {
			return false, stageErr
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
}

// stageDevice mounts one candidate read-only and copies a valid Incus payload into tmpfs.
func stageDevice(device string) (bool, error) {
	if err := syscall.Mount(device, mediaMountPath, "iso9660", syscall.MS_RDONLY, ""); err != nil {
		return false, nil
	}

	if err := validateMedia(); err != nil {
		if unmountErr := lazyUnmount(mediaMountPath); unmountErr != nil {
			return false, fmt.Errorf("unmount rejected media %s: %w", device, unmountErr)
		}

		return false, nil
	}

	if err := syscall.Mount("incus-guest-agent", stageMountPath, "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, "mode=0700,size=50M"); err != nil {
		_ = lazyUnmount(mediaMountPath)

		return false, fmt.Errorf("mount staging tmpfs: %w", err)
	}

	for _, file := range requiredMediaFiles {
		if err := copyMediaFile(file); err != nil {
			_ = lazyUnmount(stageMountPath)
			_ = lazyUnmount(mediaMountPath)

			return false, err
		}
	}

	if err := lazyUnmount(mediaMountPath); err != nil {
		_ = lazyUnmount(stageMountPath)

		return false, fmt.Errorf("unmount Incus media %s: %w", device, err)
	}

	log.Printf("staged Incus agent files from %s", device)

	return true, nil
}

// validateMedia verifies that every required payload file is regular, nonempty, and suitably executable.
func validateMedia() error {
	for _, file := range requiredMediaFiles {
		info, err := os.Lstat(filepath.Join(mediaMountPath, file.name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", file.name)
		}
		if info.Size() == 0 {
			return fmt.Errorf("%s is empty", file.name)
		}
		if file.executable && info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("%s is not executable", file.name)
		}
	}

	return nil
}

// copyMediaFile streams one validated media file into the private staging tmpfs.
func copyMediaFile(file stagedFile) error {
	sourcePath := filepath.Join(mediaMountPath, file.name)
	destinationPath := filepath.Join(stageMountPath, file.name)

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", file.name, err)
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", file.name, err)
	}

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create staged %s: %w", file.name, err)
	}

	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %s: %w", file.name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close staged %s: %w", file.name, closeErr)
	}

	return nil
}

// superviseAgent starts the Incus agent in its own process group and reaps its complete process tree.
func superviseAgent(ctx context.Context) error {
	command := exec.Command(filepath.Join(stageMountPath, "incus-agent"))
	command.Dir = stageMountPath
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := command.Start(); err != nil {
		return fmt.Errorf("start incus-agent: %w", err)
	}

	childPID := command.Process.Pid
	events := make(chan childWait, 16)
	go reapChildren(events)

	ctxDone := ctx.Done()
	var directStatus *syscall.WaitStatus
	var shutdownTimer *time.Timer
	var shutdownDeadline <-chan time.Time
	requestedShutdown := false
	killSent := false

	for {
		select {
		case <-ctxDone:
			ctxDone = nil
			requestedShutdown = true
			if err := signalProcessGroup(childPID, syscall.SIGTERM); err != nil {
				return err
			}
			shutdownTimer = time.NewTimer(shutdownGrace)
			shutdownDeadline = shutdownTimer.C
		case event := <-events:
			if errors.Is(event.err, syscall.ECHILD) {
				if shutdownTimer != nil {
					shutdownTimer.Stop()
				}
				if requestedShutdown {
					return nil
				}
				if directStatus == nil {
					return errors.New("incus-agent process tree disappeared without an exit status")
				}

				return agentExitError(*directStatus)
			}
			if event.err != nil {
				return fmt.Errorf("wait for incus-agent process tree: %w", event.err)
			}
			if event.pid == childPID {
				status := event.status
				directStatus = &status
				if shutdownTimer == nil {
					if err := signalProcessGroup(childPID, syscall.SIGTERM); err != nil {
						return err
					}
					shutdownTimer = time.NewTimer(shutdownGrace)
					shutdownDeadline = shutdownTimer.C
				}
			} else {
				log.Printf("reaped orphaned agent descendant pid=%d", event.pid)
			}
		case <-shutdownDeadline:
			if !killSent {
				if err := signalProcessGroup(childPID, syscall.SIGKILL); err != nil {
					return err
				}
				killSent = true
				shutdownTimer.Reset(killGrace)
				shutdownDeadline = shutdownTimer.C
			} else {
				return errors.New("incus-agent process tree did not exit after SIGKILL")
			}
		}
	}
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

// agentExitError describes an unexpected exit from the host-supplied agent.
func agentExitError(status syscall.WaitStatus) error {
	if status.Exited() {
		return fmt.Errorf("incus-agent exited unexpectedly with status %d", status.ExitStatus())
	}
	if status.Signaled() {
		return fmt.Errorf("incus-agent exited unexpectedly from signal %s", status.Signal())
	}

	return fmt.Errorf("incus-agent exited unexpectedly with wait status %d", status)
}
