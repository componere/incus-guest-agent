package agent_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-guest-agent/internal/agent"
	"github.com/componere/incus-guest-agent/internal/agent/mocks"
)

type serviceHarness struct {
	// finder is the generated DeviceFinder mock.
	finder *mocks.MockDeviceFinder
	// stage is the generated StageManager mock.
	stage *mocks.MockStageManager
	// proc is the generated AgentProcess mock.
	proc *mocks.MockAgentProcess
	// wait is the generated Waiter mock.
	wait *mocks.MockWaiter
	// svc is the core service under test.
	svc *agent.Service
}

// discardLogger returns a logger that drops all records.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newServiceHarness constructs a [agent.Service] wired to generated port mocks.
func newServiceHarness(t *testing.T) *serviceHarness {
	t.Helper()

	h := &serviceHarness{
		finder: mocks.NewMockDeviceFinder(t),
		stage:  mocks.NewMockStageManager(t),
		proc:   mocks.NewMockAgentProcess(t),
		wait:   mocks.NewMockWaiter(t),
	}
	h.svc = agent.New(h.finder, h.stage, h.proc, h.wait, discardLogger())

	return h
}

func TestServiceRun(t *testing.T) {
	t.Parallel()

	first := agent.DevicePath("/dev/sr0")
	second := agent.DevicePath("/dev/sr1")
	stageFailure := errors.New("stage copy failed")

	tests := []struct {
		name        string
		setup       func(t *testing.T, h *serviceHarness) context.Context
		assertError func(t *testing.T, err error)
		assertState func(t *testing.T, h *serviceHarness)
	}{
		{
			name: "no media waits then probes again",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return(nil, nil).Once()
				h.wait.EXPECT().Wait(mock.Anything).Return(nil).Once()
				h.finder.EXPECT().Find(mock.Anything).Return(nil, nil).Once()
				h.wait.EXPECT().Wait(mock.Anything).Return(context.Canceled).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.ErrorIs(t, err, agent.ErrShutdown)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNotCalled(t, "Run", mock.Anything)
				h.finder.AssertNumberOfCalls(t, "Find", 2)
				h.wait.AssertNumberOfCalls(t, "Wait", 2)
			},
		},
		{
			name: "invalid candidate progresses to the next candidate",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first, second}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(agent.ErrInvalidMedia).Once()
				h.stage.EXPECT().Stage(mock.Anything, second).Return(nil).Once()
				h.proc.EXPECT().Run(mock.Anything).Return(nil).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNumberOfCalls(t, "Run", 1)
				h.stage.AssertNumberOfCalls(t, "Stage", 2)
				h.wait.AssertNotCalled(t, "Wait", mock.Anything)
			},
		},
		{
			name: "all invalid candidates wait and retry",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first, second}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(agent.ErrInvalidMedia).Once()
				h.stage.EXPECT().Stage(mock.Anything, second).Return(agent.ErrInvalidMedia).Once()
				h.wait.EXPECT().Wait(mock.Anything).Return(nil).Once()
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(agent.ErrInvalidMedia).Once()
				h.wait.EXPECT().Wait(mock.Anything).Return(context.Canceled).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.ErrorIs(t, err, agent.ErrShutdown)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNotCalled(t, "Run", mock.Anything)
				h.finder.AssertNumberOfCalls(t, "Find", 2)
				h.wait.AssertNumberOfCalls(t, "Wait", 2)
			},
		},
		{
			name: "stage error cleans up and retries",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(4)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first, second}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(stageFailure).Once()
				h.wait.EXPECT().Wait(mock.Anything).Return(nil).Once()
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{second}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, second).Return(nil).Once()
				h.proc.EXPECT().Run(mock.Anything).Return(nil).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.stage.AssertCalled(t, "Cleanup", mock.Anything)
				h.proc.AssertNumberOfCalls(t, "Run", 1)
				assert.Equal(
					t,
					1,
					countStageCalls(h, first),
					"stage error must not continue to later candidates in the same probe",
				)
				h.wait.AssertNumberOfCalls(t, "Wait", 1)
			},
		},
		{
			name: "cancellation while waiting returns shutdown",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Once()
				h.finder.EXPECT().Find(mock.Anything).Return(nil, nil).Once()
				h.wait.EXPECT().Wait(mock.Anything).RunAndReturn(func(waitCtx context.Context) error {
					cancel()
					<-waitCtx.Done()

					return waitCtx.Err()
				}).Once()

				return ctx
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				require.ErrorIs(t, err, agent.ErrShutdown)
				assert.NotErrorIs(t, err, agent.ErrUnexpectedExit)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNotCalled(t, "Run", mock.Anything)
				h.finder.AssertNumberOfCalls(t, "Find", 1)
				h.wait.AssertNumberOfCalls(t, "Wait", 1)
			},
		},
		{
			name: "successful stage starts exactly one process",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first, second}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(nil).Once()
				h.proc.EXPECT().Run(mock.Anything).Return(nil).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNumberOfCalls(t, "Run", 1)
				h.stage.AssertNumberOfCalls(t, "Stage", 1)
				h.stage.AssertNotCalled(t, "Stage", mock.Anything, second)
				h.wait.AssertNotCalled(t, "Wait", mock.Anything)
			},
		},
		{
			name: "unexpected process exit is a failure",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(nil).Once()
				h.proc.EXPECT().Run(mock.Anything).Return(agent.ErrUnexpectedExit).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				require.ErrorIs(t, err, agent.ErrUnexpectedExit)
				assert.NotErrorIs(t, err, agent.ErrShutdown)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNumberOfCalls(t, "Run", 1)
			},
		},
		{
			name: "requested shutdown is a clean result",
			setup: func(t *testing.T, h *serviceHarness) context.Context {
				t.Helper()
				h.stage.EXPECT().Cleanup(mock.Anything).Return(nil).Times(2)
				h.finder.EXPECT().Find(mock.Anything).Return([]agent.DevicePath{first}, nil).Once()
				h.stage.EXPECT().Stage(mock.Anything, first).Return(nil).Once()
				h.proc.EXPECT().Run(mock.Anything).Return(agent.ErrShutdown).Once()

				return context.Background()
			},
			assertError: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				require.ErrorIs(t, err, agent.ErrShutdown)
				assert.NotErrorIs(t, err, agent.ErrUnexpectedExit)
			},
			assertState: func(t *testing.T, h *serviceHarness) {
				t.Helper()
				h.proc.AssertNumberOfCalls(t, "Run", 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newServiceHarness(t)
			ctx := tt.setup(t, h)

			err := h.svc.Run(ctx)

			tt.assertError(t, err)
			tt.assertState(t, h)
		})
	}
}

// countStageCalls reports how many Stage calls targeted device.
func countStageCalls(h *serviceHarness, device agent.DevicePath) int {
	var count int
	for _, call := range h.stage.Calls {
		if call.Method != "Stage" || len(call.Arguments) < 2 {
			continue
		}
		got, ok := call.Arguments.Get(1).(agent.DevicePath)
		if ok && got == device {
			count++
		}
	}

	return count
}
