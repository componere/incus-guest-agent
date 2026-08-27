//go:build linux

package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/componere/incus-guest-agent/internal/agent"
)

const (
	// runtimeDirMode is the permission used for runtime mountpoint directories.
	runtimeDirMode = 0o700
)

// Stager mounts Incus media, copies the required files into tmpfs, and
// cleans leftover runtime mounts.
type Stager struct {
	// root is the private runtime directory that holds media and staging mounts.
	root string
}

// NewStager constructs a [Stager] that uses [RuntimeRoot].
func NewStager() *Stager {
	return &Stager{root: RuntimeRoot}
}

// Stage mounts device, validates the five-file contract, and publishes the
// files into the staging tmpfs. The medium is unmounted before Stage returns
// success.
func (s *Stager) Stage(ctx context.Context, device agent.DevicePath) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureMountpoints(); err != nil {
		return err
	}

	mediaPath := s.mediaPath()
	if err := syscall.Mount(string(device), mediaPath, "iso9660", syscall.MS_RDONLY, ""); err != nil {
		return mountError(device, err)
	}

	if err := ValidateFiles(mediaPath); err != nil {
		if unmountErr := lazyUnmount(mediaPath); unmountErr != nil {
			return fmt.Errorf("unmount rejected media %s: %w", device, unmountErr)
		}

		return fmt.Errorf("%w: %w", agent.ErrInvalidMedia, err)
	}

	return s.stageValidated(device, mediaPath)
}

// Cleanup unmounts runtime mounts and removes the private runtime directory.
func (s *Stager) Cleanup(_ context.Context) error {
	var cleanupErr error
	for _, path := range []string{s.stagePath(), s.mediaPath()} {
		if err := lazyUnmount(path); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("unmount %s: %w", path, err))
		}
	}
	if err := os.RemoveAll(s.root); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove %s: %w", s.root, err))
	}

	return cleanupErr
}

// stageValidated mounts tmpfs, stream-copies the required files, and unmounts
// the validated medium.
func (s *Stager) stageValidated(device agent.DevicePath, mediaPath string) error {
	stagePath := s.stagePath()
	if err := syscall.Mount(
		TmpfsSource,
		stagePath,
		"tmpfs",
		syscall.MS_NOSUID|syscall.MS_NODEV,
		TmpfsData,
	); err != nil {
		_ = lazyUnmount(mediaPath)

		return fmt.Errorf("mount staging tmpfs: %w", err)
	}

	for _, name := range RequiredFiles() {
		if err := CopyFile(mediaPath, stagePath, name); err != nil {
			_ = lazyUnmount(stagePath)
			_ = lazyUnmount(mediaPath)

			return err
		}
	}

	if err := lazyUnmount(mediaPath); err != nil {
		_ = lazyUnmount(stagePath)

		return fmt.Errorf("unmount Incus media %s: %w", device, err)
	}

	return nil
}

// ensureMountpoints creates the media and staging directories.
func (s *Stager) ensureMountpoints() error {
	if err := os.MkdirAll(s.mediaPath(), runtimeDirMode); err != nil {
		return fmt.Errorf("create media mountpoint: %w", err)
	}
	if err := os.MkdirAll(s.stagePath(), runtimeDirMode); err != nil {
		return fmt.Errorf("create staging mountpoint: %w", err)
	}

	return nil
}

// mediaPath returns the iso9660 mountpoint under the runtime root.
func (s *Stager) mediaPath() string {
	return s.root + "/" + MediaName
}

// stagePath returns the tmpfs staging directory under the runtime root.
func (s *Stager) stagePath() string {
	return s.root + "/" + StageName
}

// lazyUnmount detaches a mount while accepting a missing or unmounted path.
func lazyUnmount(path string) error {
	err := syscall.Unmount(path, syscall.MNT_DETACH)
	if err == nil || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) {
		return nil
	}

	return err
}

// mountError maps a failed iso9660 mount onto either [agent.ErrInvalidMedia]
// or a retryable preparation error.
func mountError(device agent.DevicePath, err error) error {
	if isAbsentMedia(err) {
		return fmt.Errorf("%w: mount %s: %w", agent.ErrInvalidMedia, device, err)
	}

	return fmt.Errorf("mount %s: %w", device, err)
}

// isAbsentMedia reports whether err means the candidate has no usable medium.
func isAbsentMedia(err error) bool {
	return errors.Is(err, syscall.ENOMEDIUM) ||
		errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.ENXIO) ||
		errors.Is(err, syscall.ENODEV) ||
		errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.EIO)
}
