package util

import "sync"

// FailureCounter tracks consecutive failures per key with "increment on
// failure, reset to zero on success, cap once a threshold is reached"
// semantics. It is safe for concurrent use.
//
// It exists so components that need bounded-retry bookkeeping (circuit
// breakers, health monitors, ...) share a single, tested implementation of
// the pattern instead of hand-writing map access and counters.
type FailureCounter[K comparable] struct {
	mu     sync.Mutex
	counts map[K]int
}

// NewFailureCounter creates an empty failure counter.
func NewFailureCounter[K comparable]() *FailureCounter[K] {
	return &FailureCounter[K]{counts: make(map[K]int)}
}

// Get returns the current consecutive-failure count for key (0 when unknown).
func (fc *FailureCounter[K]) Get(key K) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.counts[key]
}

// Increment records one more consecutive failure for key and returns the new count.
func (fc *FailureCounter[K]) Increment(key K) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.counts[key]++
	return fc.counts[key]
}

// Reset clears the consecutive-failure count for key, returning the count
// recorded before the reset so callers can log the transition.
func (fc *FailureCounter[K]) Reset(key K) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	previous := fc.counts[key]
	delete(fc.counts, key)
	return previous
}

// Exceeded reports whether the consecutive-failure count for key has reached
// max. A max of zero or less is treated as "no cap" and always returns false.
func (fc *FailureCounter[K]) Exceeded(key K, max int) bool {
	if max <= 0 {
		return false
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.counts[key] >= max
}

// positiveNumber constrains PositiveOrDefault to numeric types where "unset"
// is conventionally expressed as a zero or negative value. time.Duration
// satisfies it through its ~int64 underlying type.
type positiveNumber interface {
	~int | ~int64 | ~float64
}

// PositiveOrDefault returns value when it is strictly positive and fallback
// otherwise. It captures the recurring "if configured value <= 0, use the
// package default" idiom in one place.
func PositiveOrDefault[T positiveNumber](value, fallback T) T {
	if value <= 0 {
		return fallback
	}
	return value
}
