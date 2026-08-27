package linux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinderFindExcludesNonBlockDevices(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "sr2", "not-a-device", 0o644)
	writeFile(t, dir, "sr0", "not-a-device", 0o644)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sr10"), 0o700))
	writeFile(t, dir, "sda", "not-optical", 0o644)

	finder := NewFinderGlob(filepath.Join(dir, "sr*"))
	got, err := finder.Find(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFinderFindHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := NewFinderGlob(filepath.Join(t.TempDir(), "sr*")).Find(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
}
