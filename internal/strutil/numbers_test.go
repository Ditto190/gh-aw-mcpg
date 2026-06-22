package strutil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterfaceToIntString(t *testing.T) {
	t.Parallel()

	t.Run("float64 integer", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(float64(42))
		assert.True(t, ok)
		assert.Equal(t, "42", s)
	})

	t.Run("float64 zero", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(float64(0))
		assert.True(t, ok)
		assert.Equal(t, "0", s)
	})

	t.Run("float64 negative integer", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(float64(-7))
		assert.True(t, ok)
		assert.Equal(t, "-7", s)
	})

	t.Run("float64 non-integer returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(float64(1.5))
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("float64 truncatable decimal returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(float64(123.9))
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("float64 out of int64 range returns false", func(t *testing.T) {
		t.Parallel()
		// 1e20 exceeds int64 max; explicit out-of-range guard rejects it
		s, ok := InterfaceToIntString(float64(1e20))
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("json.Number integer", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(json.Number("999"))
		assert.True(t, ok)
		assert.Equal(t, "999", s)
	})

	t.Run("json.Number leading zeros canonicalized", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(json.Number("00123"))
		assert.True(t, ok)
		assert.Equal(t, "123", s)
	})

	t.Run("json.Number large value within int64", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(json.Number("9223372036854775807"))
		assert.True(t, ok)
		assert.Equal(t, "9223372036854775807", s)
	})

	t.Run("json.Number decimal returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(json.Number("123.45"))
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("json.Number out of int64 range returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(json.Number("99999999999999999999"))
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("string returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString("42")
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("int returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(42)
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})

	t.Run("nil returns false", func(t *testing.T) {
		t.Parallel()
		s, ok := InterfaceToIntString(nil)
		assert.False(t, ok)
		assert.Equal(t, "", s)
	})
}
