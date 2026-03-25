package syncutil_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/syncutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetOrCreate_ReturnsCachedValue verifies that a pre-populated cache entry
// is returned without invoking create.
func TestGetOrCreate_ReturnsCachedValue(t *testing.T) {
	var mu sync.RWMutex
	cache := map[string]int{"key": 42}

	createCalled := false
	v, err := syncutil.GetOrCreate(&mu, cache, "key", func() (int, error) {
		createCalled = true
		return 99, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 42, v)
	assert.False(t, createCalled, "create should not be called when value is already cached")
}

// TestGetOrCreate_CallsCreateForMissingKey verifies that create is called and
// the result is stored when the key is absent.
func TestGetOrCreate_CallsCreateForMissingKey(t *testing.T) {
	var mu sync.RWMutex
	cache := map[string]int{}

	v, err := syncutil.GetOrCreate(&mu, cache, "key", func() (int, error) {
		return 7, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 7, v)
	assert.Equal(t, 7, cache["key"], "value should be stored in cache")
}

// TestGetOrCreate_DoesNotStoreOnError verifies that a failed create does not
// pollute the cache.
func TestGetOrCreate_DoesNotStoreOnError(t *testing.T) {
	var mu sync.RWMutex
	cache := map[string]int{}
	boom := errors.New("boom")

	v, err := syncutil.GetOrCreate(&mu, cache, "key", func() (int, error) {
		return 0, boom
	})

	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 0, v)
	_, exists := cache["key"]
	assert.False(t, exists, "failed value should not be stored in cache")
}

// TestGetOrCreate_CreateCalledOnce verifies the double-check locking ensures
// create is called exactly once even under concurrent access.
func TestGetOrCreate_CreateCalledOnce(t *testing.T) {
	var mu sync.RWMutex
	cache := map[string]int{}

	var createCount atomic.Int32
	const numGoroutines = 100

	var wg sync.WaitGroup
	results := make([]int, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			v, err := syncutil.GetOrCreate(&mu, cache, "key", func() (int, error) {
				createCount.Add(1)
				return 42, nil
			})
			if err == nil {
				results[idx] = v
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), createCount.Load(), "create should be called exactly once")
	for i, v := range results {
		assert.Equal(t, 42, v, "goroutine %d got unexpected value", i)
	}
	assert.Equal(t, 42, cache["key"])
}

// TestGetOrCreate_MultipleKeys verifies independent keys are handled separately.
func TestGetOrCreate_MultipleKeys(t *testing.T) {
	var mu sync.RWMutex
	cache := map[string]string{}

	for _, key := range []string{"a", "b", "c"} {
		k := key
		v, err := syncutil.GetOrCreate(&mu, cache, k, func() (string, error) {
			return "value-" + k, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "value-"+k, v)
	}

	assert.Len(t, cache, 3)
}

// TestGetOrCreate_RaceDetector is run with -race to verify no data races.
func TestGetOrCreate_RaceDetector(t *testing.T) {
	var mu sync.RWMutex
	cache := map[int]int{}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		key := i % 3 // intentional key collisions
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			_, _ = syncutil.GetOrCreate(&mu, cache, k, func() (int, error) {
				return k * 10, nil
			})
		}(key)
	}
	wg.Wait()
}
