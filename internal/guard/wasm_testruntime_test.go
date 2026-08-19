package guard

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// newTestWasmRuntime creates an interpreter-backed wazero runtime for tests.
// The interpreter avoids JIT compilation, which keeps the many short-lived
// runtimes used by unit tests fast to create and tear down.
//
// Centralizing runtime construction here keeps test configuration consistent
// and makes future wazero config changes a single-line edit.
func newTestWasmRuntime(ctx context.Context) wazero.Runtime {
	return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
}

// newTestWasmModuleConfig returns the module configuration used when tests
// instantiate raw WASM binaries directly.
func newTestWasmModuleConfig(name string) wazero.ModuleConfig {
	return wazero.NewModuleConfig().WithName(name)
}

// instantiateTestWasmModule instantiates wasmBytes in rt and registers module
// cleanup with t.
func instantiateTestWasmModule(t *testing.T, ctx context.Context, rt wazero.Runtime, wasmBytes []byte, name string) api.Module {
	t.Helper()
	mod, err := rt.InstantiateWithConfig(ctx, wasmBytes, newTestWasmModuleConfig(name))
	require.NoError(t, err, "failed to instantiate WASM module %s", name)
	return mod
}

// setupTestWasmGuard instantiates a WASM module directly (bypassing
// NewWasmGuardWithOptions) and returns a WasmGuard wired to it plus a cleanup
// function that closes the module and runtime.
func setupTestWasmGuard(t *testing.T, wasmBytes []byte, name string) (*WasmGuard, func()) {
	t.Helper()
	ctx := context.Background()
	rt := newTestWasmRuntime(ctx)
	mod := instantiateTestWasmModule(t, ctx, rt, wasmBytes, name)
	g := &WasmGuard{name: name, module: mod}
	return g, func() {
		require.NoError(t, mod.Close(ctx))
		require.NoError(t, rt.Close(ctx))
	}
}
