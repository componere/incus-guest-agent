//go:build linux

package linux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/componere/incus-guest-agent/internal/agent"
)

func TestFinderFindReturnsLexicalBlockDevices(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sr1 := filepath.Join(dir, "sr1")
	sr0 := filepath.Join(dir, "sr0")
	sr10 := filepath.Join(dir, "sr10")
	regular := filepath.Join(dir, "sr2")
	writeFile(t, dir, "sr2", "regular", 0o644)

	for _, path := range []string{sr1, sr0, sr10} {
		err := unix.Mknod(path, unix.S_IFBLK|0o600, int(unix.Mkdev(11, 0)))
		if err != nil {
			t.Skipf("cannot create block device nodes: %v", err)
		}
	}

	info, err := os.Lstat(regular)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())

	finder := NewFinderGlob(filepath.Join(dir, "sr*"))
	got, err := finder.Find(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []agent.DevicePath{
		agent.DevicePath(sr0),
		agent.DevicePath(sr1),
		agent.DevicePath(sr10),
	}, got)
}
