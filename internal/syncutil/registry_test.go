package syncutil_test

import (
	"sync"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/syncutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryCRUD(t *testing.T) {
	registry := syncutil.NewRegistry[string, int]()

	registry.Set("one", 1)
	registry.Set("two", 2)

	value, ok := registry.Get("one")
	require.True(t, ok)
	assert.Equal(t, 1, value)
	assert.True(t, registry.Has("one"))
	assert.ElementsMatch(t, []string{"one", "two"}, registry.Keys())
	assert.Equal(t, 2, registry.Len())

	registry.Remove("one")
	_, ok = registry.Get("one")
	assert.False(t, ok)
	assert.False(t, registry.Has("one"))
	assert.Equal(t, 1, registry.Len())
}

func TestRegistryRange(t *testing.T) {
	registry := syncutil.NewRegistry[string, int]()
	registry.Set("one", 1)
	registry.Set("two", 2)

	entries := map[string]int{}
	registry.Range(func(key string, value int) bool {
		entries[key] = value
		return true
	})

	assert.Equal(t, map[string]int{"one": 1, "two": 2}, entries)
}

func TestRegistryGetOrCreate(t *testing.T) {
	registry := syncutil.NewRegistry[string, int]()
	registry.Set("existing", 1)

	createCalled := false
	assert.Equal(t, 1, registry.GetOrCreate("existing", func() int {
		createCalled = true
		return 2
	}))
	assert.False(t, createCalled)
	assert.Equal(t, 2, registry.GetOrCreate("new", func() int { return 2 }))
	assert.Equal(t, 2, registry.Len())
}

func TestRegistryGetOrCreateConcurrent(t *testing.T) {
	registry := syncutil.NewRegistry[string, int]()
	const goroutines = 100

	var createCount int
	var createMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			assert.Equal(t, 1, registry.GetOrCreate("key", func() int {
				createMu.Lock()
				defer createMu.Unlock()
				createCount++
				return createCount
			}))
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, createCount)
}
