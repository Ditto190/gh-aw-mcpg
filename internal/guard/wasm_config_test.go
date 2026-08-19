package guard

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// TestNewGuardModuleConfig verifies the shared module config builder applies the
// stdin/stdout isolation guarantees and the default module name.
func TestNewGuardModuleConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("uses provided name", func(t *testing.T) {
		rt := newTestWasmRuntime(ctx)
		defer func() { require.NoError(t, rt.Close(ctx)) }()

		mod, err := rt.InstantiateWithConfig(ctx, minimalGuardWasm, newGuardModuleConfig("named-guard", &bytes.Buffer{}, &bytes.Buffer{}))
		require.NoError(t, err)
		defer func() { require.NoError(t, mod.Close(ctx)) }()

		assert.Equal(t, "named-guard", mod.Name())
	})

	t.Run("empty name falls back to guard", func(t *testing.T) {
		rt := newTestWasmRuntime(ctx)
		defer func() { require.NoError(t, rt.Close(ctx)) }()

		mod, err := rt.InstantiateWithConfig(ctx, minimalGuardWasm, newGuardModuleConfig("", &bytes.Buffer{}, &bytes.Buffer{}))
		require.NoError(t, err)
		defer func() { require.NoError(t, mod.Close(ctx)) }()

		assert.Equal(t, "guard", mod.Name())
	})
}

// TestNewGuardRuntimeConfig verifies the shared runtime config builder produces a
// usable runtime for each compilation cache selection path.
func TestNewGuardRuntimeConfig(t *testing.T) {
	ctx := context.Background()

	cacheDir := t.TempDir()
	customCache, err := wazero.NewCompilationCacheWithDir(cacheDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, customCache.Close(ctx)) }()

	testCases := []struct {
		name string
		opts *WasmGuardOptions
	}{
		{name: "nil options uses global cache", opts: nil},
		{name: "custom cache", opts: &WasmGuardOptions{CompilationCache: customCache}},
		{name: "cache disabled", opts: &WasmGuardOptions{DisableCompilationCache: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := newGuardRuntimeConfig(tc.opts)
			require.NotNil(t, config)

			rt := wazero.NewRuntimeWithConfig(ctx, config)
			defer func() { require.NoError(t, rt.Close(ctx)) }()

			mod, err := rt.InstantiateWithConfig(ctx, minimalGuardWasm, newGuardModuleConfig("config-guard", &bytes.Buffer{}, &bytes.Buffer{}))
			require.NoError(t, err)
			require.NoError(t, mod.Close(ctx))
		})
	}
}
