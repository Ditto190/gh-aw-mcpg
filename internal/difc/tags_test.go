package difc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringsToTags(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected []Tag
	}{
		{
			name:     "empty slice",
			values:   []string{},
			expected: []Tag{},
		},
		{
			name:     "nil slice",
			values:   nil,
			expected: []Tag{},
		},
		{
			name:     "single value",
			values:   []string{"private:owner"},
			expected: []Tag{"private:owner"},
		},
		{
			name:     "multiple values",
			values:   []string{"private:owner", "private:owner/repo"},
			expected: []Tag{"private:owner", "private:owner/repo"},
		},
		{
			name:     "trims whitespace",
			values:   []string{"  private:owner  ", "\tprivate:repo\t"},
			expected: []Tag{"private:owner", "private:repo"},
		},
		{
			name:     "skips empty strings",
			values:   []string{"private:owner", "", "  ", "private:repo"},
			expected: []Tag{"private:owner", "private:repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringsToTags(tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTagsToStrings(t *testing.T) {
	tests := []struct {
		name     string
		tags     []Tag
		expected []string
	}{
		{
			name:     "empty slice",
			tags:     []Tag{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			tags:     nil,
			expected: []string{},
		},
		{
			name:     "single tag",
			tags:     []Tag{"private:owner"},
			expected: []string{"private:owner"},
		},
		{
			name:     "multiple tags",
			tags:     []Tag{"private:owner", "private:owner/repo"},
			expected: []string{"private:owner", "private:owner/repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TagsToStrings(tt.tags)
			assert.Equal(t, tt.expected, result)
		})
	}
}
