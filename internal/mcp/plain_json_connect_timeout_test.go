package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryPlainJSONTransportHonorsConnectTimeout(t *testing.T) {
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-unblock
	}))
	defer server.Close()
	defer close(unblock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connectTimeout := 50 * time.Millisecond

	start := time.Now()
	conn, err := tryPlainJSONTransport(
		ctx,
		cancel,
		"slow-plain-json",
		server.URL,
		nil,
		server.Client(),
		connectTimeout,
	)

	require.Error(t, err)
	assert.Nil(t, conn)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}
