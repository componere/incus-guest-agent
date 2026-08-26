package linux

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ValidateFiles checks that dir contains the five required Incus guest-agent
// files as nonempty regular files, with an executable bit on incus-agent.
func ValidateFiles(dir string) error {
	for _, name := range RequiredFiles() {
		if err := validateFile(dir, name); err != nil {
			return err
		}
	}

	return nil
}

// validateFile verifies one required media file under dir.
func validateFile(dir string, name string) error {
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", name)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", name)
	}
	if name == agentBinaryName && info.Mode().Perm()&executableBits == 0 {
		return fmt.Errorf("%s is not executable", name)
	}

	return nil
}

// CopyFile streams name from srcDir to dstDir through a temporary file.
//
// Permission bits are preserved. The final path is replaced only after the
// copy is synced and closed, so a failed attempt never leaves a usable
// partial destination.
func CopyFile(srcDir string, dstDir string, name string) error {
	source, err := os.Open(filepath.Join(srcDir, name))
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}

	tempPath := filepath.Join(dstDir, name+copyTempSuffix)
	destination, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create staged %s: %w", name, err)
	}

	_, copyErr := io.Copy(destination, source)
	syncErr := destination.Sync()
	closeErr := destination.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tempPath)

		return copyFailure(name, copyErr, syncErr, closeErr)
	}

	if err := os.Chmod(tempPath, info.Mode().Perm()); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("preserve mode for %s: %w", name, err)
	}
	if err := os.Rename(tempPath, filepath.Join(dstDir, name)); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("publish staged %s: %w", name, err)
	}

	return nil
}

// copyFailure joins stream, sync, and close errors from a failed copy.
func copyFailure(name string, copyErr error, syncErr error, closeErr error) error {
	switch {
	case copyErr != nil:
		return fmt.Errorf("copy %s: %w", name, copyErr)
	case syncErr != nil:
		return fmt.Errorf("sync staged %s: %w", name, syncErr)
	default:
		return fmt.Errorf("close staged %s: %w", name, closeErr)
	}
}
