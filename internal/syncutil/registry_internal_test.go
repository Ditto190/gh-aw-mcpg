// Package syncutil tests – internal test file to allow direct access to the
// unexported Registry.mu field, needed to deterministically force the
// write-locked double-check branch in GetOrCreate (see
// TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate below).
package syncutil

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate deterministically
// exercises the write-locked double-check branch in GetOrCreate by waiting
// until both goroutines observe the initial miss before allowing either to
// acquire the write lock.
func TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	registry := NewRegistry[string, int]()

	var createCount atomic.Int32
	missed := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan int, 2)

	for range 2 {
		go func() {
			results <- registry.getOrCreate("key", func() int {
				createCount.Add(1)
				return 42
			}, func() {
				missed <- struct{}{}
				<-release
			})
		}()
	}

	for i := 0; i < 2; i++ {
		select {
		case <-missed:
		case <-time.After(time.Second):
			require.FailNow("timed out waiting for both goroutines to observe the initial miss")
		}
	}

	close(release)

	for i := 0; i < 2; i++ {
		select {
		case v := <-results:
			assert.Equal(42, v)
		case <-time.After(time.Second):
			require.FailNow("timed out waiting for goroutine result")
		}
	}

	assert.Equal(int32(1), createCount.Load(),
		"create must be called exactly once; the double-check must prevent the second goroutine from calling it")
	v, ok := registry.Get("key")
	require.True(ok)
	assert.Equal(42, v)
}
