package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/guard"
)

func configureWasmCompilationCache(ctx context.Context, flagChanged bool, flagValue, effectiveLogDir string, warn func(string, ...interface{})) (string, error) {
	resolvedDir := resolveWasmCacheDir(flagChanged, flagValue, effectiveLogDir)
	debugLog.Printf("Configuring WASM compilation cache: resolvedDir=%q, flagChanged=%v", resolvedDir, flagChanged)

	if err := guard.ConfigureGlobalCompilationCache(ctx, resolvedDir); err == nil {
		if resolvedDir == "" {
			debugLog.Print("WASM compilation cache configured: mode=in-memory")
		} else {
			debugLog.Printf("WASM compilation cache configured: mode=disk, dir=%q", resolvedDir)
		}
		return resolvedDir, nil
	} else if resolvedDir == "" {
		return "", fmt.Errorf("failed to configure WASM compilation cache: %w", err)
	} else {
		debugLog.Printf("Disk-backed WASM cache failed, falling back to in-memory: dir=%q, err=%v", resolvedDir, err)
		if warn != nil {
			warn("Falling back to in-memory WASM compilation cache after %q failed: %v", resolvedDir, err)
		}
		if fallbackErr := guard.ConfigureGlobalCompilationCache(ctx, ""); fallbackErr != nil {
			return "", errors.Join(
				fmt.Errorf("failed to configure WASM compilation cache at %q: %w", resolvedDir, err),
				fmt.Errorf("failed to configure in-memory WASM compilation cache fallback: %w", fallbackErr),
			)
		}
		debugLog.Print("WASM compilation cache fallback configured: mode=in-memory")
		return "", nil
	}
}
