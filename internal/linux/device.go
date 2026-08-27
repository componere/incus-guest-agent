package linux

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/componere/incus-guest-agent/internal/agent"
)

// Finder enumerates lexically ordered optical block devices.
type Finder struct {
	// pattern is the glob used to discover candidate device paths.
	pattern string
}

// NewFinder constructs a [Finder] for [DeviceGlob].
func NewFinder() *Finder {
	return NewFinderGlob(DeviceGlob)
}

// NewFinderGlob constructs a [Finder] for an explicit glob pattern.
func NewFinderGlob(pattern string) *Finder {
	return &Finder{pattern: pattern}
}

// Find lists matching block devices without mounting them.
func (f *Finder) Find(ctx context.Context) ([]agent.DevicePath, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(f.pattern)
	if err != nil {
		return nil, err
	}
	slices.Sort(matches)

	devices := make([]agent.DevicePath, 0, len(matches))
	for _, match := range matches {
		if !isBlockDevice(match) {
			continue
		}
		devices = append(devices, agent.DevicePath(match))
	}

	return devices, nil
}

// isBlockDevice reports whether path names an existing block device.
func isBlockDevice(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}

	mode := info.Mode()

	return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
}
