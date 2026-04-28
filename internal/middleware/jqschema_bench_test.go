package middleware

import (
	"context"
	"testing"

	"github.com/itchyny/gojq"
)

// BenchmarkApplyJqSchema_CompiledCode benchmarks the current implementation
// that uses pre-compiled query code (the optimized version)
func BenchmarkApplyJqSchema_CompiledCode(b *testing.B) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "small object",
			input: map[string]interface{}{"name": "test", "count": 42, "active": true},
		},
		{
			name: "medium object",
			input: map[string]interface{}{
				"total_count": 1000,
				"items": []interface{}{
					map[string]interface{}{"id": 1, "name": "item1", "price": 10.5},
					map[string]interface{}{"id": 2, "name": "item2", "price": 20.5},
					map[string]interface{}{"id": 3, "name": "item3", "price": 30.5},
				},
			},
		},
		{
			name: "large nested object",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id":       123,
					"login":    "testuser",
					"verified": true,
					"profile": map[string]interface{}{
						"bio":      "Test bio",
						"location": "Test location",
						"website":  "https://example.com",
					},
				},
				"repositories": []interface{}{
					map[string]interface{}{
						"id":          1,
						"name":        "repo1",
						"stars":       100,
						"description": "First repo",
						"owner": map[string]interface{}{
							"login": "owner1",
							"id":    999,
						},
					},
					map[string]interface{}{
						"id":          2,
						"name":        "repo2",
						"stars":       200,
						"description": "Second repo",
						"owner": map[string]interface{}{
							"login": "owner2",
							"id":    888,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := applyJqSchema(ctx, tt.input)
				if err != nil {
					b.Fatalf("applyJqSchema failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkApplyJqSchema_ParseEveryTime used to benchmark parsing the query on every
// invocation to quantify the compile-once speedup.
//
// NOTE: This benchmark is no longer valid. Since walk_schema is now a native Go
// function registered via gojq.WithFunction, running the parsed query without the
// corresponding function registration will fail at runtime with an undefined-function
// error. Skipping to avoid misleading benchmark output.
func BenchmarkApplyJqSchema_ParseEveryTime(b *testing.B) {
	b.Skip("invalid benchmark: walk_schema requires gojq.Compile with gojq.WithFunction registration; parse-only path no longer produces meaningful results")
}

// BenchmarkCompileVsParse compares the time to compile vs parse the jq query
func BenchmarkCompileVsParse(b *testing.B) {
	b.Run("parse_only", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := gojq.Parse(jqSchemaFilter)
			if err != nil {
				b.Fatalf("Parse failed: %v", err)
			}
		}
	})

	b.Run("parse_and_compile", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			query, err := gojq.Parse(jqSchemaFilter)
			if err != nil {
				b.Fatalf("Parse failed: %v", err)
			}
			_, err = gojq.Compile(query)
			if err != nil {
				b.Fatalf("Compile failed: %v", err)
			}
		}
	})
}

// BenchmarkPreviewCreation benchmarks the preview string creation for large payloads.
// The optimized version slices the byte slice before converting to string, avoiding
// a full allocation of the entire (potentially multi-MB) payload as a string.
func BenchmarkPreviewCreation(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1 * 1024 * 1024},
	}

	for _, tt := range sizes {
		// Build a payload larger than PayloadPreviewSize
		payload := make([]byte, tt.size)
		for i := range payload {
			payload[i] = 'x'
		}

		b.Run("optimized/"+tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Optimized: slice bytes before converting to string
				_ = string(payload[:PayloadPreviewSize]) + "..."
			}
		})

		b.Run("original/"+tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Original: convert full payload to string first
				s := string(payload)
				_ = s[:PayloadPreviewSize] + "..."
			}
		})
	}
}
