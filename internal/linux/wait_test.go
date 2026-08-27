package linux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaiterWaitReturnsAfterDuration(t *testing.T) {
	t.Parallel()

	wait := NewWaiter(time.Millisecond)
	err := wait.Wait(context.Background())
	require.NoError(t, err)
}

func TestWaiterWaitHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewWaiter(time.Hour).Wait(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
