package syncutil

import "sync"

// Registry is a thread-safe key-value registry.
type Registry[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]V
}

// NewRegistry creates an empty Registry.
func NewRegistry[K comparable, V any]() *Registry[K, V] {
	return &Registry[K, V]{
		entries: make(map[K]V),
	}
}

// Set stores value under key.
func (r *Registry[K, V]) Set(key K, value V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = value
}

// Get retrieves the value stored under key.
func (r *Registry[K, V]) Get(key K) (V, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.entries[key]
	return value, ok
}

// GetOrCreate returns the value for key, creating and storing it when absent.
// create is called while the registry write lock is held.
func (r *Registry[K, V]) GetOrCreate(key K, create func() V) V {
	r.mu.RLock()
	value, ok := r.entries[key]
	r.mu.RUnlock()
	if ok {
		return value
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok = r.entries[key]; ok {
		return value
	}

	value = create()
	r.entries[key] = value
	return value
}

// Remove deletes the value stored under key.
func (r *Registry[K, V]) Remove(key K) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}

// Keys returns a snapshot of all registered keys.
func (r *Registry[K, V]) Keys() []K {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]K, 0, len(r.entries))
	for key := range r.entries {
		keys = append(keys, key)
	}
	return keys
}

// Entries returns a snapshot of all registered key-value pairs.
func (r *Registry[K, V]) Entries() map[K]V {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make(map[K]V, len(r.entries))
	for key, value := range r.entries {
		entries[key] = value
	}
	return entries
}

// Len returns the number of registered entries.
func (r *Registry[K, V]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
