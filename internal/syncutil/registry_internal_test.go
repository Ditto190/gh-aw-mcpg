// Package syncutil tests – internal test file to allow direct access to the
// unexported Registry.mu field, needed to deterministically force the
// write-locked double-check branch in GetOrCreate (see
// TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate below).
package syncutil

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate deterministically
// exercises the write-locked double-check branch in GetOrCreate: the test
// itself holds the write lock so both goroutines are guaranteed to observe a
// cache miss under the read lock, then race for the write lock. The winner
// calls create() while the loser queues behind the write lock; once the
// winner stores the value and releases the lock, the loser must find the key
// already populated on its double-check and must NOT invoke create a second
// time.
func TestRegistryGetOrCreateDoubleCheckPreventsRedundantCreate(t *testing.T) {
	registry := NewRegistry[string, int]()

	// Hold the write lock before starting goroutines so both are guaranteed
	// to block at mu.RLock() and observe a cache miss once released.
	registry.mu.Lock()

	var createCount atomic.Int32
	firstCreate := make(chan struct{})
	createEntered := make(chan struct{})
	start := make(chan struct{})
	callStarted := make(chan struct{}, 2)
	results := make(chan int, 2)
	var createEnteredOnce sync.Once
	var wg sync.WaitGroup

	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			<-start
			callStarted <- struct{}{}
			v := registry.GetOrCreate("key", func() int {
				// Only the goroutine that wins the write lock reaches here.
				// It blocks on firstCreate so the other goroutine is forced
				// to queue on the write lock and hit the double-check.
				createCount.Add(1)
				createEnteredOnce.Do(func() { close(createEntered) })
				<-firstCreate
				return 42
			})
			results <- v
		}()
	}

	close(start)
	for i := 0; i < 2; i++ {
		select {
		case <-callStarted:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for goroutines to start GetOrCreate")
		}
	}

	// Release the test's write lock. Both goroutines unblock from mu.RLock(),
	// observe a cache miss, release the read lock, and race for the write lock.
	registry.mu.Unlock()

	select {
	case <-createEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for create() to be entered")
	}

	close(firstCreate)
	wg.Wait()

	for i := 0; i < 2; i++ {
		select {
		case v := <-results:
			assert.Equal(t, 42, v)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for goroutine result")
		}
	}

	assert.Equal(t, int32(1), createCount.Load(),
		"create must be called exactly once; the double-check must prevent the second goroutine from calling it")
	v, ok := registry.Get("key")
	require.True(t, ok)
	assert.Equal(t, 42, v)
}
