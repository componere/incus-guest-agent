package linux

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPayload     = "incus-guest-agent-test-payload"
	testExecMode    = 0o755
	testRegularMode = 0o640
)

// mediaFixture holds a temporary directory used as Incus media.
type mediaFixture struct {
	// dir is the temporary media directory.
	dir string
}

// newMediaFixture creates an empty temporary media directory.
func newMediaFixture(t *testing.T) *mediaFixture {
	t.Helper()

	return &mediaFixture{dir: t.TempDir()}
}

// writeRequired writes the five required media files with valid contents and modes.
func (f *mediaFixture) writeRequired(t *testing.T) {
	t.Helper()

	for _, name := range RequiredFiles() {
		mode := os.FileMode(testRegularMode)
		if name == agentBinaryName {
			mode = testExecMode
		}
		writeFile(t, f.dir, name, testPayload, mode)
	}
}

// writeFile creates name under dir with contents and the exact permission bits.
func writeFile(t *testing.T, dir string, name string, contents string, mode os.FileMode) {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), mode))
	require.NoError(t, os.Chmod(path, mode))
}

func TestValidateFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(t *testing.T, dir string)
		wantErr string
	}{
		{
			name: "accepts the complete five-file contract",
		},
		{
			name: "rejects a missing required file",
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dir, fileServerCrt)))
			},
			wantErr: "stat server.crt",
		},
		{
			name: "rejects a zero-length file",
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, fileAgentConf, "", testRegularMode)
			},
			wantErr: "agent.conf is empty",
		},
		{
			name: "rejects a non-regular file",
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				path := filepath.Join(dir, fileAgentKey)
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Mkdir(path, 0o700))
			},
			wantErr: "agent.key is not a regular file",
		},
		{
			name: "rejects a symlink in place of a required file",
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				path := filepath.Join(dir, fileAgentCrt)
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink("agent.conf", path))
			},
			wantErr: "agent.crt is not a regular file",
		},
		{
			name: "rejects incus-agent without an executable bit",
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, agentBinaryName, testPayload, testRegularMode)
			},
			wantErr: "incus-agent is not executable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newMediaFixture(t)
			fx.writeRequired(t)
			if tt.mutate != nil {
				tt.mutate(t, fx.dir)
			}

			err := ValidateFiles(fx.dir)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	t.Run("preserves permission bits", func(t *testing.T) {
		t.Parallel()

		src := t.TempDir()
		dst := t.TempDir()
		writeFile(t, src, agentBinaryName, testPayload, testExecMode)
		writeFile(t, src, fileAgentConf, testPayload, testRegularMode)

		require.NoError(t, CopyFile(src, dst, agentBinaryName))
		require.NoError(t, CopyFile(src, dst, fileAgentConf))

		assertCopiedFile(t, dst, agentBinaryName, testPayload, testExecMode)
		assertCopiedFile(t, dst, fileAgentConf, testPayload, testRegularMode)
		_, tempErr := os.Lstat(filepath.Join(dst, agentBinaryName+copyTempSuffix))
		require.ErrorIs(t, tempErr, os.ErrNotExist)
		_, tempErr = os.Lstat(filepath.Join(dst, fileAgentConf+copyTempSuffix))
		require.ErrorIs(t, tempErr, os.ErrNotExist)
	})

	t.Run("failed exclusive temp leaves existing final file", func(t *testing.T) {
		t.Parallel()

		src := t.TempDir()
		dst := t.TempDir()
		existing := []byte("keep-me")
		writeFile(t, src, fileAgentConf, testPayload, testRegularMode)
		writeFile(t, dst, fileAgentConf, string(existing), testRegularMode)
		writeFile(t, dst, fileAgentConf+copyTempSuffix, "stale-temp", testRegularMode)

		err := CopyFile(src, dst, fileAgentConf)
		require.Error(t, err)

		got, readErr := os.ReadFile(filepath.Join(dst, fileAgentConf))
		require.NoError(t, readErr)
		assert.Equal(t, existing, got)
		stale, staleErr := os.ReadFile(filepath.Join(dst, fileAgentConf+copyTempSuffix))
		require.NoError(t, staleErr)
		assert.Equal(t, []byte("stale-temp"), stale)
	})
}

// assertCopiedFile checks that name exists under dir with contents and mode.
func assertCopiedFile(t *testing.T, dir string, name string, contents string, mode os.FileMode) {
	t.Helper()

	info, err := os.Lstat(filepath.Join(dir, name))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, mode, info.Mode().Perm())
	got, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	assert.Equal(t, contents, string(got))
}
