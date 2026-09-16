package util

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFailureCounter_IncrementResetGet(t *testing.T) {
	assert := assert.New(t)

	fc := NewFailureCounter[string]()
	assert.Equal(0, fc.Get("a"))

	assert.Equal(1, fc.Increment("a"))
	assert.Equal(2, fc.Increment("a"))
	assert.Equal(1, fc.Increment("b"))

	assert.Equal(2, fc.Get("a"))
	assert.Equal(1, fc.Get("b"))

	assert.Equal(2, fc.Reset("a"))
	assert.Equal(0, fc.Get("a"))
	assert.Equal(1, fc.Get("b"), "resetting one key must not affect others")
	assert.Equal(0, fc.Reset("unknown"))
}

func TestFailureCounter_Exceeded(t *testing.T) {
	assert := assert.New(t)

	fc := NewFailureCounter[string]()
	assert.False(fc.Exceeded("a", 2))

	fc.Increment("a")
	assert.False(fc.Exceeded("a", 2))

	fc.Increment("a")
	assert.True(fc.Exceeded("a", 2))

	fc.Reset("a")
	assert.False(fc.Exceeded("a", 2))

	fc.Increment("a")
	assert.False(fc.Exceeded("a", 0), "non-positive max means no cap")
	assert.False(fc.Exceeded("a", -1), "non-positive max means no cap")
}

func TestFailureCounter_ConcurrentAccess(t *testing.T) {
	fc := NewFailureCounter[string]()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fc.Increment("shared")
			fc.Get("shared")
			fc.Exceeded("shared", 10)
		}()
	}
	wg.Wait()

	assert.Equal(t, 50, fc.Get("shared"))
}

func TestPositiveOrDefault(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(5, PositiveOrDefault(5, 3))
	assert.Equal(3, PositiveOrDefault(0, 3))
	assert.Equal(3, PositiveOrDefault(-1, 3))

	assert.Equal(time.Minute, PositiveOrDefault(time.Minute, 30*time.Second))
	assert.Equal(30*time.Second, PositiveOrDefault(time.Duration(0), 30*time.Second))
	assert.Equal(30*time.Second, PositiveOrDefault(-time.Second, 30*time.Second))

	assert.InDelta(1.5, PositiveOrDefault(1.5, 2.5), 1e-9)
	assert.InDelta(2.5, PositiveOrDefault(0.0, 2.5), 1e-9)
}
