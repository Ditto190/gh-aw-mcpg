package mcpresult

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsErrorResult(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		want bool
	}{
		{"isError true", map[string]interface{}{"isError": true}, true},
		{"isError false", map[string]interface{}{"isError": false}, false},
		{"isError absent", map[string]interface{}{"content": []interface{}{}}, false},
		{"isError wrong type", map[string]interface{}{"isError": "yes"}, false},
		{"empty map", map[string]interface{}{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsErrorResult(tc.m))
		})
	}
}
