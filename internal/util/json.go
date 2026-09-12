package util

import (
	"encoding/json"
	"errors"
	"io"
)

// DeepCloneJSON creates a deep copy of a JSON-compatible value.
// It handles the three container types used by encoding/json:
// map[string]any (JSON objects), []any (JSON arrays),
// and any other type (JSON scalars: string, float64, bool, nil), which is
// returned as-is since scalar values are not reference types and need no cloning.
func DeepCloneJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		clone := make(map[string]any, len(val))
		for k, v := range val {
			clone[k] = DeepCloneJSON(v)
		}
		return clone
	case []any:
		clone := make([]any, len(val))
		for i, v := range val {
			clone[i] = DeepCloneJSON(v)
		}
		return clone
	default:
		return v
	}
}

// ErrTrailingJSON is returned by DecodeStrictJSON when the input contains more
// than one JSON value. Callers can use errors.Is to distinguish trailing data
// from a malformed first value and wrap it with a domain-specific message.
var ErrTrailingJSON = errors.New("input must contain exactly one JSON value")

// DecodeStrictJSON decodes exactly one JSON value from r into value.
// Unknown fields are rejected, and any data following the first JSON value
// (other than whitespace) is rejected as well. Decode failures are returned
// unwrapped so callers can add their own context; trailing data is reported as
// ErrTrailingJSON.
func DecodeStrictJSON(r io.Reader, value any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrTrailingJSON
		}
		return err
	}
	return nil
}
