package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "help long flag",
			args:       []string{"--help"},
			wantStatus: 0,
			wantStdout: "Usage: incus-guest-agent [--help] [--version]\n",
		},
		{
			name:       "help short flag",
			args:       []string{"-h"},
			wantStatus: 0,
			wantStdout: "Usage: incus-guest-agent [--help] [--version]\n",
		},
		{
			name:       "version long flag",
			args:       []string{"--version"},
			wantStatus: 0,
			wantStdout: "incus-guest-agent dev\n",
		},
		{
			name:       "version short flag",
			args:       []string{"-v"},
			wantStatus: 0,
			wantStdout: "incus-guest-agent dev\n",
		},
		{
			name:       "unknown argument",
			args:       []string{"--unknown"},
			wantStatus: 2,
			wantStderr: "incus-guest-agent: unknown argument \"--unknown\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			status := run(tt.args, &stdout, &stderr)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			if tt.wantStatus == 0 {
				require.Empty(t, stderr.String())
			}
		})
	}
}
