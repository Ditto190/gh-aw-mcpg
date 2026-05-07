package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/guard"
)

func configureWasmCompilationCache(ctx context.Context, flagChanged bool, flagValue, effectiveLogDir string, warn func(string, ...interface{})) (string, error) {
	resolvedDir := resolveWasmCacheDir(flagChanged, flagValue, effectiveLogDir)
	if err := guard.ConfigureGlobalCompilationCache(ctx, resolvedDir); err == nil {
		return resolvedDir, nil
	} else if resolvedDir == "" {
		return "", fmt.Errorf("failed to configure WASM compilation cache: %w", err)
	} else {
		if warn != nil {
			warn("Falling back to in-memory WASM compilation cache after %q failed: %v", resolvedDir, err)
		}
		if fallbackErr := guard.ConfigureGlobalCompilationCache(ctx, ""); fallbackErr != nil {
			return "", errors.Join(
				fmt.Errorf("failed to configure WASM compilation cache at %q: %w", resolvedDir, err),
				fmt.Errorf("failed to configure in-memory WASM compilation cache fallback: %w", fallbackErr),
			)
		}
		return "", nil
	}
}
