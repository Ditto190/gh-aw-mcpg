package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortedSetKeys(t *testing.T) {
	t.Parallel()

	t.Run("returns sorted keys", func(t *testing.T) {
		t.Parallel()
		set := map[string]struct{}{"banana": {}, "apple": {}, "cherry": {}}
		assert.Equal(t, []string{"apple", "banana", "cherry"}, SortedSetKeys(set))
	})

	t.Run("returns empty slice for empty set", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, SortedSetKeys(map[string]struct{}{}))
	})

	t.Run("returns single element slice", func(t *testing.T) {
		t.Parallel()
		set := map[string]struct{}{"only": {}}
		assert.Equal(t, []string{"only"}, SortedSetKeys(set))
	})

	t.Run("handles nil map", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, SortedSetKeys(nil))
	})
}

func TestGetStringFromMap(t *testing.T) {
	t.Parallel()

	t.Run("returns string value when present", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"owner": "octo"}
		assert.Equal(t, "octo", GetStringFromMap(m, "owner"))
	})

	t.Run("returns empty string for missing key", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"owner": "octo"}
		assert.Equal(t, "", GetStringFromMap(m, "repo"))
	})

	t.Run("returns empty string for non-string value", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"number": float64(1)}
		assert.Equal(t, "", GetStringFromMap(m, "number"))
	})

	t.Run("returns empty string for nil map", func(t *testing.T) {
		t.Parallel()
		var m map[string]any
		assert.Equal(t, "", GetStringFromMap(m, "owner"))
	})

	t.Run("returns empty string when value is empty string", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"owner": ""}
		assert.Equal(t, "", GetStringFromMap(m, "owner"))
	})

	t.Run("variadic: returns first non-empty match", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"htmlUrl": "https://example.com"}
		assert.Equal(t, "https://example.com", GetStringFromMap(m, "html_url", "htmlUrl"))
	})

	t.Run("variadic: first key takes priority when both present", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"html_url": "https://first.com", "htmlUrl": "https://second.com"}
		assert.Equal(t, "https://first.com", GetStringFromMap(m, "html_url", "htmlUrl"))
	})

	t.Run("variadic: skips empty first key and returns second", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"html_url": "", "htmlUrl": "https://second.com"}
		assert.Equal(t, "https://second.com", GetStringFromMap(m, "html_url", "htmlUrl"))
	})

	t.Run("variadic: all keys missing returns empty string", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"other": "value"}
		assert.Equal(t, "", GetStringFromMap(m, "html_url", "htmlUrl"))
	})

	t.Run("variadic: skips non-string first key and returns second", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"html_url": 99, "htmlUrl": "https://second.com"}
		assert.Equal(t, "https://second.com", GetStringFromMap(m, "html_url", "htmlUrl"))
	})

	t.Run("no keys returns empty string", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"owner": "octo"}
		assert.Equal(t, "", GetStringFromMap(m))
	})
}

func TestDeduplicateStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		sorted   bool
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			sorted:   false,
			expected: []string{},
		},
		{
			name:     "empty input",
			input:    []string{},
			sorted:   false,
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"a"},
			sorted:   false,
			expected: []string{"a"},
		},
		{
			name:     "no duplicates unsorted",
			input:    []string{"c", "a", "b"},
			sorted:   false,
			expected: []string{"c", "a", "b"},
		},
		{
			name:     "no duplicates sorted",
			input:    []string{"c", "a", "b"},
			sorted:   true,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "removes duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			sorted:   false,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "removes duplicates sorted",
			input:    []string{"b", "a", "b", "c", "a"},
			sorted:   true,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "trims whitespace",
			input:    []string{"  a  ", "\tb\t", " c"},
			sorted:   false,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "trims and deduplicates",
			input:    []string{" a", "a ", "a"},
			sorted:   false,
			expected: []string{"a"},
		},
		{
			name:     "skips empty strings",
			input:    []string{"", "a", "", "b"},
			sorted:   false,
			expected: []string{"a", "b"},
		},
		{
			name:     "skips whitespace-only strings",
			input:    []string{"   ", "a", "\t", "b"},
			sorted:   false,
			expected: []string{"a", "b"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			sorted:   false,
			expected: []string{"a"},
		},
		{
			name:     "all empty",
			input:    []string{"", "  ", "\t"},
			sorted:   false,
			expected: []string{},
		},
		{
			name:     "preserves first-seen order without sort",
			input:    []string{"z", "m", "a", "m", "z"},
			sorted:   false,
			expected: []string{"z", "m", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeduplicateStrings(tt.input, tt.sorted)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindDuplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		items     []int
		duplicate int
		found     bool
	}{
		{name: "nil items"},
		{name: "unique items", items: []int{1, 2, 3}},
		{name: "finds first duplicate", items: []int{1, 2, 1, 2}, duplicate: 1, found: true},
		{name: "finds zero duplicate", items: []int{1, 0, 2, 0}, duplicate: 0, found: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			duplicate, found := FindDuplicate(tt.items)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.duplicate, duplicate)
		})
	}
}

func TestStringsToAny(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty (non-nil) slice", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []any{}, StringsToAny(nil))
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, StringsToAny([]string{}))
	})

	t.Run("converts all entries preserving order", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []any{"octo", "hub", "bot"}, StringsToAny([]string{"octo", "hub", "bot"}))
	})
}

func TestCopyTrimmedStringIntMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]int
		expected map[string]int
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map returns nil",
			input:    map[string]int{},
			expected: nil,
		},
		{
			name:     "single entry with clean key",
			input:    map[string]int{"key": 42},
			expected: map[string]int{"key": 42},
		},
		{
			name:     "leading whitespace trimmed from key",
			input:    map[string]int{"  key": 10},
			expected: map[string]int{"key": 10},
		},
		{
			name:     "trailing whitespace trimmed from key",
			input:    map[string]int{"key  ": 10},
			expected: map[string]int{"key": 10},
		},
		{
			name:     "leading and trailing whitespace trimmed from key",
			input:    map[string]int{"  key  ": 7},
			expected: map[string]int{"key": 7},
		},
		{
			name:     "tab characters trimmed from key",
			input:    map[string]int{"\tkey\t": 3},
			expected: map[string]int{"key": 3},
		},
		{
			name:     "newline characters trimmed from key",
			input:    map[string]int{"\nkey\n": 5},
			expected: map[string]int{"key": 5},
		},
		{
			name:     "internal whitespace preserved in key",
			input:    map[string]int{"hello world": 1},
			expected: map[string]int{"hello world": 1},
		},
		{
			name:     "multiple entries all trimmed",
			input:    map[string]int{"  a  ": 1, " b ": 2, "c": 3},
			expected: map[string]int{"a": 1, "b": 2, "c": 3},
		},
		{
			name:     "zero value preserved",
			input:    map[string]int{"key": 0},
			expected: map[string]int{"key": 0},
		},
		{
			name:     "negative value preserved",
			input:    map[string]int{"key": -5},
			expected: map[string]int{"key": -5},
		},
		{
			name:     "large positive value preserved",
			input:    map[string]int{"key": 1<<31 - 1},
			expected: map[string]int{"key": 1<<31 - 1},
		},
		{
			name:     "mixed clean and whitespace keys",
			input:    map[string]int{"clean": 1, "  padded  ": 2},
			expected: map[string]int{"clean": 1, "padded": 2},
		},
		{
			name:     "whitespace-only key becomes empty string key",
			input:    map[string]int{"   ": 99},
			expected: map[string]int{"": 99},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := CopyTrimmedStringIntMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyTrimmedStringIntMap_IsDefensiveCopy(t *testing.T) {
	t.Parallel()

	original := map[string]int{"key": 1}
	copied := CopyTrimmedStringIntMap(original)
	require.NotNil(t, copied)

	// Modifying the copy should not affect the original
	copied["key"] = 999
	assert.Equal(t, 1, original["key"], "original map should not be affected by changes to copy")

	// Modifying the original should not affect the copy
	original["key"] = 42
	assert.Equal(t, 999, copied["key"], "copy should not be affected by changes to original")
}

func TestCopyTrimmedStringIntMap_NilAndEmptyReturnNil(t *testing.T) {
	t.Parallel()

	// Both nil and empty input produce nil output
	nilResult := CopyTrimmedStringIntMap(nil)
	emptyResult := CopyTrimmedStringIntMap(map[string]int{})

	assert.Nil(t, nilResult, "nil input should return nil")
	assert.Nil(t, emptyResult, "empty map input should return nil")
}
