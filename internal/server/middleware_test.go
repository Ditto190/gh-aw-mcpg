package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildDefaultHandlerConfig(t *testing.T) {
	unifiedServer := &UnifiedServer{}
	sessionTimeout := 15 * time.Minute

	cfg := buildDefaultHandlerConfig(unifiedServer, sessionTimeout, defaultHandlerConfigOptions{
		handlerLog: logSDK,
		logTag:     "unified",
		apiKeys:    []string{"test-api-key"},
		hmacSecret: "test-hmac-secret",
	})

	require.Same(t, logSDK, cfg.handlerLog)
	require.Equal(t, sessionTimeout, cfg.sessionTimeout)
	require.Equal(t, "unified", cfg.logTag)
	require.Same(t, unifiedServer, cfg.unifiedServer)
	require.Equal(t, []string{"test-api-key"}, cfg.apiKeys)
	require.Equal(t, "test-hmac-secret", cfg.hmacSecret)
}
