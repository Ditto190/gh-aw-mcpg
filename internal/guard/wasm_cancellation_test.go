package guard

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/sys"
)

// TestWasmExecContext verifies that guard executions are detached from caller
// cancellation and refused outright when the caller is already gone.
func TestWasmExecContext(t *testing.T) {
	t.Run("nil context falls back to background", func(t *testing.T) {
		//nolint:staticcheck // deliberately passing a nil context to exercise the fallback
		execCtx, err := wasmExecContext(nil)
		require.NoError(t, err)
		require.NotNil(t, execCtx)
		assert.NoError(t, execCtx.Err())
	})

	t.Run("already canceled context is refused", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		execCtx, err := wasmExecContext(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, execCtx)
	})

	t.Run("expired deadline is refused", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		<-ctx.Done()

		execCtx, err := wasmExecContext(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, execCtx)
	})

	t.Run("caller cancellation does not propagate to the execution context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		execCtx, err := wasmExecContext(ctx)
		require.NoError(t, err)

		// Cancelling the caller (e.g. an HTTP client that disconnects) must not
		// cancel the in-flight guard call, otherwise wazero would close the
		// module shared by every other caller.
		cancel()
		assert.NoError(t, execCtx.Err(), "guard execution context must survive caller cancellation")
	})

	t.Run("context values are preserved", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), testCtxKey, "test-value")

		execCtx, err := wasmExecContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "test-value", execCtx.Value(testCtxKey))
	})
}

// TestCallWasmGuardFunctionCanceledCaller verifies that a caller which gives up
// while queued behind the guard mutex neither enters the module nor poisons the
// guard for subsequent callers.
func TestCallWasmGuardFunctionCanceledCaller(t *testing.T) {
	g, cleanup := setupTestWasmGuard(t, labelAgentSuccessWasm, "canceled-caller-guard")
	defer cleanup()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := g.callWasmGuardFunction(canceledCtx, "label_agent", &mockBackendCaller{}, map[string]any{})
	require.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "aborted before WASM execution")

	// The guard must remain usable: one disconnected client must not disable the
	// shared module for the rest of the session.
	assert.True(t, g.IsHealthy(), "guard must stay healthy after a canceled caller")

	result, err := g.callWasmGuardFunction(context.Background(), "label_agent", &mockBackendCaller{}, map[string]any{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"difc_mode": "strict"}`, string(result))
}

// TestIsWasmTrapHostCancellation verifies that host-side teardown exit codes are
// not treated as guest traps. wazero reports module closure caused by context
// cancellation or deadline expiry as a sys.ExitError with a reserved exit code;
// poisoning the guard for those would disable it for all callers.
func TestIsWasmTrapHostCancellation(t *testing.T) {
	t.Run("context canceled exit code is not a trap", func(t *testing.T) {
		assert.False(t, isWasmTrap(sys.NewExitError(sys.ExitCodeContextCanceled)))
	})

	t.Run("deadline exceeded exit code is not a trap", func(t *testing.T) {
		assert.False(t, isWasmTrap(sys.NewExitError(sys.ExitCodeDeadlineExceeded)))
	})
}
