package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildDefaultHandlerConfig(t *testing.T) {
	unifiedServer := &UnifiedServer{}
	sessionTimeout := 15 * time.Minute

	cfg := buildDefaultHandlerConfig(unifiedServer, "test-api-key", "test-hmac-secret", sessionTimeout, logSDK, "unified")

	require.Same(t, logSDK, cfg.handlerLog)
	require.Equal(t, sessionTimeout, cfg.sessionTimeout)
	require.Equal(t, "unified", cfg.logTag)
	require.Same(t, unifiedServer, cfg.unifiedServer)
	require.Equal(t, "test-api-key", cfg.apiKey)
	require.Equal(t, "test-hmac-secret", cfg.hmacSecret)
}
