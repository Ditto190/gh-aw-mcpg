package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for validateAgainstCustomSchema and validateCustomServerConfig that cover
// branches not exercised by the existing T-CFG-010 through T-CFG-014 tests.

// TestValidateAgainstCustomSchema_FetchFailure covers the fetchAndFixSchema error path
// (lines 207-215 in validation.go) when the schema server returns a non-200 status.
func TestValidateAgainstCustomSchema_FetchFailure(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to fetch custom schema")
	assert.ErrorContains(t, err, "mytype")
}

// TestValidateAgainstCustomSchema_UnreachableURL covers the fetchAndFixSchema connection
// error path when the schema URL is completely unreachable (server is already closed).
func TestValidateAgainstCustomSchema_UnreachableURL(t *testing.T) {
	// Create and immediately close a server to get an address that refuses connections
	closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := closedServer.URL
	closedServer.Close()

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, unreachableURL, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to fetch custom schema")
}

// TestValidateAgainstCustomSchema_SchemaWithDifferentID covers the branch at lines 248-257
// where the schema's $id differs from the fetch URL. In this case both the fetch URL and
// the $id URL must be registered with the compiler.
func TestValidateAgainstCustomSchema_SchemaWithDifferentID(t *testing.T) {
	const customSchemaID = "https://schemas.example.com/mytype-v1.json"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id":     customSchemaID,
			"type":    "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
				},
				"container": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"type", "container"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	// The fetch URL differs from the schema's $id: compilation uses the $id
	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL, "mcpServers.test-server")

	// Should pass validation because required fields are present
	assert.NoError(t, err, "schema with $id different from fetch URL should validate successfully")
}

// TestValidateAgainstCustomSchema_SchemaWithDifferentID_MissingRequired verifies that
// schema validation still fails correctly when the schema has a different $id and the
// server config is missing a required field.
func TestValidateAgainstCustomSchema_SchemaWithDifferentID_MissingRequired(t *testing.T) {
	const customSchemaID = "https://schemas.example.com/mytype-v2.json"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id":     customSchemaID,
			"type":    "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
				},
				"container": map[string]interface{}{
					"type": "string",
				},
				"requiredField": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"type", "container", "requiredField"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	// Missing requiredField - validation should fail
	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
		// No AdditionalProperties with "requiredField"
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "does not match custom schema")
}

func TestValidateAgainstCustomSchema_CachesSchemaByURL(t *testing.T) {
	var requestCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type":    "object",
			"properties": map[string]interface{}{
				"type":      map[string]interface{}{"type": "string"},
				"container": map[string]interface{}{"type": "string"},
			},
			"required": []string{"type", "container"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	require.NoError(t, validateAgainstCustomSchema("test-server-1", server, mockServer.URL, "mcpServers.test-server-1"))
	mockServer.Close()
	require.NoError(t, validateAgainstCustomSchema("test-server-2", server, mockServer.URL, "mcpServers.test-server-2"))

	assert.Equal(t, int32(1), requestCount.Load())
}

// TestValidateAgainstCustomSchema_AdditionalPropertiesMerged verifies that fields stored
// in AdditionalProperties (custom fields from JSON unmarshaling) are merged into the
// validation map before schema validation (lines 297-299 in validation.go).
func TestValidateAgainstCustomSchema_AdditionalPropertiesMerged(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type":    "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
				},
				"container": map[string]interface{}{
					"type": "string",
				},
				"customField": map[string]interface{}{
					"type": "string",
				},
				"anotherCustomField": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"type", "container", "customField", "anotherCustomField"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	// AdditionalProperties are set directly (simulating JSON unmarshal of custom fields)
	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
		AdditionalProperties: map[string]interface{}{
			"customField":        "custom-value",
			"anotherCustomField": "another-value",
		},
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL, "mcpServers.test-server")

	assert.NoError(t, err, "AdditionalProperties should be merged into validation map")
}

// TestValidateAgainstCustomSchema_AdditionalPropertiesMissingRequired verifies that
// when AdditionalProperties are missing a required custom field, validation fails.
func TestValidateAgainstCustomSchema_AdditionalPropertiesMissingRequired(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type":    "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
				},
				"container": map[string]interface{}{
					"type": "string",
				},
				"mandatoryCustomField": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"type", "container", "mandatoryCustomField"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	// AdditionalProperties is populated but missing mandatoryCustomField
	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
		AdditionalProperties: map[string]interface{}{
			"someOtherField": "value",
		},
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "does not match custom schema")
}

// TestValidateCustomServerConfig_NonStringSchemaValue covers the type assertion branch
// (lines 183-187 in validation.go) where the custom schema map value is not a string.
// When the schema value is not a string, schemaURL is set to "" and validation is skipped.
func TestValidateCustomServerConfig_NonStringSchemaValue(t *testing.T) {
	tests := []struct {
		name        string
		schemaValue interface{}
	}{
		{
			name:        "integer_schema_value",
			schemaValue: 42,
		},
		{
			name:        "map_schema_value",
			schemaValue: map[string]interface{}{"url": "https://example.com"},
		},
		{
			name:        "bool_schema_value",
			schemaValue: true,
		},
		{
			name:        "slice_schema_value",
			schemaValue: []string{"https://example.com/schema.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customSchemas := map[string]interface{}{
				"mytype": tt.schemaValue,
			}

			server := &StdinServerConfig{
				Type:      "mytype",
				Container: "ghcr.io/example/mytype:latest",
			}

			// Non-string values cause schemaURL to be "" which skips validation
			err := validateCustomServerConfig("test-server", server, customSchemas, "mcpServers.test-server")

			assert.NoError(t, err, "non-string schema value should skip validation")
		})
	}
}

// TestValidateCustomServerConfig_NilCustomSchemas covers the nil customSchemas path
// (lines 171-174 in validation.go) via validateCustomServerConfig directly.
func TestValidateCustomServerConfig_NilCustomSchemas(t *testing.T) {
	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateCustomServerConfig("test-server", server, nil, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "not registered in customSchemas")
}

// TestValidateCustomServerConfig_UnregisteredType covers the case where customSchemas
// is not nil but the server type is not registered (lines 176-180 in validation.go).
func TestValidateCustomServerConfig_UnregisteredType(t *testing.T) {
	customSchemas := map[string]interface{}{
		"othertype": "https://example.com/othertype-schema.json",
	}

	server := &StdinServerConfig{
		Type:      "mytype", // not in customSchemas
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateCustomServerConfig("test-server", server, customSchemas, "mcpServers.test-server")

	require.Error(t, err)
	assert.ErrorContains(t, err, "not registered in customSchemas")
	assert.ErrorContains(t, err, "mytype")
}

// TestValidateAgainstCustomSchema_CacheHitWrongType covers the branch where the
// schema URL is found in the cache but the cached value is not a *jsonschema.Schema.
// In that case the cached value is ignored and the schema is re-fetched.
// (validation.go lines ~231-237)
func TestValidateAgainstCustomSchema_CacheHitWrongType(t *testing.T) {
	// We need a unique URL so it doesn't collide with other tests' cache entries.
	var requestCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type":    "object",
			"properties": map[string]interface{}{
				"type":      map[string]interface{}{"type": "string"},
				"container": map[string]interface{}{"type": "string"},
			},
			"required": []string{"type", "container"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	// Use a unique URL (including a path) so it doesn't collide with other tests' cache entries.
	schemaURL := mockServer.URL + "/cache-hit-wrong-type"
	t.Cleanup(func() { customSchemaCache.Delete(schemaURL) })

	// Seed the cache with a non-*jsonschema.Schema value at this URL.
	customSchemaCache.Store(schemaURL, "unexpected-string-value")

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	// The wrong-type cache hit should be ignored; the schema should be re-fetched.
	err := validateAgainstCustomSchema("test-server", server, schemaURL, "mcpServers.test-server")

	require.NoError(t, err, "validation should succeed after ignoring the wrong-type cache entry")
	assert.Equal(t, int32(1), requestCount.Load(), "schema should be fetched once after ignoring the bad cache entry")
}

func TestValidateAgainstCustomSchema_RemoteRefIsResolved(t *testing.T) {
	// httptest serves plain HTTP, so relax the HTTPS-only policy for remote refs.
	allowInsecureSchemaRefsForTest(t)

	var defsRequestCount atomic.Int32

	mockServer := newRemoteRefSchemaServer(t, &defsRequestCount)
	defer mockServer.Close()

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL+"/root.json", "mcpServers.test-server")

	require.NoError(t, err, "schema compilation should resolve remote $ref via configured loader")
	assert.Equal(t, int32(1), defsRequestCount.Load(), "remote referenced schema should be fetched once")

	// The constraints defined in the referenced document must be enforced: `container`
	// is required there, so omitting it fails validation using the cached schema.
	invalidServer := &StdinServerConfig{Type: "mytype"}
	err = validateAgainstCustomSchema("test-server", invalidServer, mockServer.URL+"/root.json", "mcpServers.test-server")

	require.Error(t, err, "constraints from the referenced schema should be enforced")
	assert.Contains(t, err.Error(), "container", "error should mention the missing required property")
	assert.Equal(t, int32(1), defsRequestCount.Load(), "cached compiled schema should not refetch the dependency")
}

func TestValidateAgainstCustomSchema_RemoteRefRequiresHTTPS(t *testing.T) {
	var defsRequestCount atomic.Int32

	mockServer := newRemoteRefSchemaServer(t, &defsRequestCount)
	defer mockServer.Close()

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL+"/root.json", "mcpServers.test-server")

	require.Error(t, err, "insecure remote $ref should be rejected")
	assert.Contains(t, err.Error(), "must use HTTPS", "error should explain the HTTPS-only policy")
	assert.Equal(t, int32(0), defsRequestCount.Load(), "insecure dependency should never be fetched")
}

// allowInsecureSchemaRefsForTest relaxes the HTTPS-only remote $ref policy for the
// duration of a test so httptest's HTTP server can serve schema fixtures.
func allowInsecureSchemaRefsForTest(t *testing.T) {
	t.Helper()
	original := requireHTTPSSchemaURLs
	requireHTTPSSchemaURLs = false
	t.Cleanup(func() { requireHTTPSSchemaURLs = original })
}

// newRemoteRefSchemaServer serves a root schema whose $ref points at a second document
// that requires the `container` field.
func newRemoteRefSchemaServer(t *testing.T, defsRequestCount *atomic.Int32) *httptest.Server {
	t.Helper()

	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root.json":
			schema := map[string]interface{}{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id":     mockServer.URL + "/root.json",
				"$ref":    "./defs.json#/definitions/server",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(schema)
		case "/defs.json":
			defsRequestCount.Add(1)
			schema := map[string]interface{}{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id":     mockServer.URL + "/defs.json",
				"definitions": map[string]interface{}{
					"server": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"type":      map[string]interface{}{"type": "string"},
							"container": map[string]interface{}{"type": "string"},
						},
						"required": []string{"type", "container"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(schema)
		default:
			http.NotFound(w, r)
		}
	}))
	return mockServer
}

func TestValidateAgainstCustomSchema_RemoteRefBudgetIsEnforced(t *testing.T) {
	// httptest serves plain HTTP, so relax the HTTPS-only policy for remote refs.
	allowInsecureSchemaRefsForTest(t)

	var requestCount atomic.Int32

	// Serve an unbounded chain of documents: /chain/0.json → /chain/1.json → …
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		next := r.URL.Path[len("/chain/"):]
		index, err := strconv.Atoi(next[:len(next)-len(".json")])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		schema := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id":     fmt.Sprintf("%s/chain/%d.json", mockServer.URL, index),
			"$ref":    fmt.Sprintf("%s/chain/%d.json", mockServer.URL, index+1),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(schema)
	}))
	defer mockServer.Close()

	server := &StdinServerConfig{
		Type:      "mytype",
		Container: "ghcr.io/example/mytype:latest",
	}

	err := validateAgainstCustomSchema("test-server", server, mockServer.URL+"/chain/0.json", "mcpServers.test-server")

	require.Error(t, err, "an unbounded $ref chain must be rejected")
	assert.Contains(t, err.Error(), "too many remote schema references", "error should report the exhausted budget")
	// One direct fetch of the root document plus at most the loader document budget.
	assert.LessOrEqual(t, requestCount.Load(), int32(maxRemoteRefDocuments+1), "loader must stop fetching once the budget is exhausted")
}
