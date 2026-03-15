package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for fetchAndFixSchema schema transformation logic.
//
// fetchAndFixSchema applies three transformations to a raw schema JSON before
// compilation to work around JSON Schema Draft 7 limitations:
//
//  1. customServerConfig.type: replaces negative-lookahead pattern with not/enum
//  2. customSchemas.patternProperties: removes negative-lookahead key, adds simple key
//  3. stdioServerConfig / httpServerConfig: injects registry and guard-policies fields
//
// Existing tests (validate_against_custom_schema_test.go) exercise the HTTP-level
// paths (non-200, unreachable) using simple mock schemas that lack the above
// structures.  These tests exercise the three transformation branches specifically.

// schemaServer is a test helper that returns an HTTP test server serving the given schema.
// It avoids calling require/t.FailNow inside the handler goroutine (unsafe in Go <1.21).
func schemaServer(t *testing.T, schema map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(schema); err != nil {
			t.Errorf("schemaServer: failed to encode schema: %v", err)
		}
	}))
}

// unmarshalSchema is a test helper that unmarshals fetchAndFixSchema output.
func unmarshalSchema(t *testing.T, schemaBytes []byte) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(schemaBytes, &result))
	return result
}

// TestFetchAndFixSchema_InvalidJSONResponse covers the json.Unmarshal failure path
// (validation_schema.go lines 113-115) when the server returns malformed JSON.
func TestFetchAndFixSchema_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("this is not valid JSON {{{"))
	}))
	defer srv.Close()

	_, err := fetchAndFixSchema(srv.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse schema")
}

// TestFetchAndFixSchema_TransformCustomServerConfigType covers lines 121-138:
// the customServerConfig.type transformation that removes the negative-lookahead
// pattern and adds a not/enum constraint instead.
func TestFetchAndFixSchema_TransformCustomServerConfigType(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": map[string]interface{}{
			"customServerConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":    "string",
						"pattern": `^(?!stdio$|http$).*`,
					},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)

	defs := got["definitions"].(map[string]interface{})
	csConf := defs["customServerConfig"].(map[string]interface{})
	props := csConf["properties"].(map[string]interface{})
	typeField := props["type"].(map[string]interface{})

	// pattern and type keys should have been removed
	_, hasPattern := typeField["pattern"]
	assert.False(t, hasPattern, "pattern key should be removed from customServerConfig.type")
	_, hasType := typeField["type"]
	assert.False(t, hasType, "type key should be removed from customServerConfig.type")

	// not/enum constraint should have been injected
	notConstraint, hasNot := typeField["not"]
	require.True(t, hasNot, "not constraint should be added to customServerConfig.type")
	notMap := notConstraint.(map[string]interface{})
	enumSlice := notMap["enum"].([]interface{})
	assert.Contains(t, enumSlice, "stdio", "not.enum should exclude 'stdio'")
	assert.Contains(t, enumSlice, "http", "not.enum should exclude 'http'")
}

// TestFetchAndFixSchema_CustomServerConfigType_MissingSubStructures verifies that the
// transformation is a no-op when intermediate keys (properties, type) are absent.
func TestFetchAndFixSchema_CustomServerConfigType_MissingSubStructures(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]interface{}
	}{
		{
			name: "customServerConfig has no properties key",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"customServerConfig": map[string]interface{}{
						"type": "object",
						// no "properties"
					},
				},
			},
		},
		{
			name: "customServerConfig.properties has no type key",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"customServerConfig": map[string]interface{}{
						"properties": map[string]interface{}{
							"container": map[string]interface{}{"type": "string"},
							// no "type"
						},
					},
				},
			},
		},
		{
			name: "definitions present but no customServerConfig",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"otherConfig": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "no definitions key at all",
			schema: map[string]interface{}{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type":    "object",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := schemaServer(t, tt.schema)
			defer srv.Close()

			result, err := fetchAndFixSchema(srv.URL)

			require.NoError(t, err, "missing structures should not cause errors")
			assert.NotEmpty(t, result, "should return non-empty bytes")
		})
	}
}

// TestFetchAndFixSchema_TransformCustomSchemasPatternProperties covers lines 140-156:
// the patternProperties transformation that removes the negative-lookahead key and
// replaces it with the simple "^[a-z][a-z0-9-]*$" key.
func TestFetchAndFixSchema_TransformCustomSchemasPatternProperties(t *testing.T) {
	customTypeDef := map[string]interface{}{
		"type":        "string",
		"description": "URL to the custom type schema",
	}
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"properties": map[string]interface{}{
			"customSchemas": map[string]interface{}{
				"type": "object",
				"patternProperties": map[string]interface{}{
					`^(?!stdio$|http$)[a-z][a-z0-9-]*$`: customTypeDef,
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)

	topProps := got["properties"].(map[string]interface{})
	customSchemas := topProps["customSchemas"].(map[string]interface{})
	patternProps := customSchemas["patternProperties"].(map[string]interface{})

	// original negative-lookahead key should be gone
	_, hasNegLookahead := patternProps[`^(?!stdio$|http$)[a-z][a-z0-9-]*$`]
	assert.False(t, hasNegLookahead, "negative-lookahead pattern key should be removed")

	// simple replacement key should be present with the original value
	simpleVal, hasSimple := patternProps["^[a-z][a-z0-9-]*$"]
	require.True(t, hasSimple, "simple pattern key should be added as replacement")
	simpleMap := simpleVal.(map[string]interface{})
	assert.Equal(t, "string", simpleMap["type"], "replacement value should preserve original definition")
}

// TestFetchAndFixSchema_CustomSchemasPatternProperties_NoNegativeLookahead verifies
// that the patternProperties transformation is skipped when no negative-lookahead key
// is present.
func TestFetchAndFixSchema_CustomSchemasPatternProperties_NoNegativeLookahead(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"customSchemas": map[string]interface{}{
				"patternProperties": map[string]interface{}{
					"^[a-z][a-z0-9-]*$": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)
	topProps := got["properties"].(map[string]interface{})
	customSchemas := topProps["customSchemas"].(map[string]interface{})
	patternProps := customSchemas["patternProperties"].(map[string]interface{})

	// existing simple key should remain untouched
	_, hasSimple := patternProps["^[a-z][a-z0-9-]*$"]
	assert.True(t, hasSimple, "existing simple pattern key should be preserved when no negative-lookahead present")
}

// TestFetchAndFixSchema_AddRegistryAndGuardPoliciesToStdioConfig covers lines 179-184:
// the injection of registry and guard-policies into stdioServerConfig.properties.
func TestFetchAndFixSchema_AddRegistryAndGuardPoliciesToStdioConfig(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": map[string]interface{}{
			"stdioServerConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"container": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)
	defs := got["definitions"].(map[string]interface{})
	stdioConf := defs["stdioServerConfig"].(map[string]interface{})
	props := stdioConf["properties"].(map[string]interface{})

	// registry field should have been added
	registry, hasRegistry := props["registry"]
	require.True(t, hasRegistry, "registry field should be added to stdioServerConfig.properties")
	registryMap := registry.(map[string]interface{})
	assert.Equal(t, "string", registryMap["type"], "registry.type should be 'string'")
	assert.NotEmpty(t, registryMap["description"], "registry.description should be set")

	// guard-policies field should have been added
	guardPolicies, hasGuardPolicies := props["guard-policies"]
	require.True(t, hasGuardPolicies, "guard-policies field should be added to stdioServerConfig.properties")
	gpMap := guardPolicies.(map[string]interface{})
	assert.Equal(t, "object", gpMap["type"], "guard-policies.type should be 'object'")
	assert.Equal(t, true, gpMap["additionalProperties"], "guard-policies.additionalProperties should be true")
}

// TestFetchAndFixSchema_AddRegistryAndGuardPoliciesToHttpConfig covers lines 186-191:
// the injection of registry and guard-policies into httpServerConfig.properties.
func TestFetchAndFixSchema_AddRegistryAndGuardPoliciesToHttpConfig(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": map[string]interface{}{
			"httpServerConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)
	defs := got["definitions"].(map[string]interface{})
	httpConf := defs["httpServerConfig"].(map[string]interface{})
	props := httpConf["properties"].(map[string]interface{})

	// registry and guard-policies should have been added
	_, hasRegistry := props["registry"]
	assert.True(t, hasRegistry, "registry field should be added to httpServerConfig.properties")

	_, hasGuardPolicies := props["guard-policies"]
	assert.True(t, hasGuardPolicies, "guard-policies field should be added to httpServerConfig.properties")

	// original fields should be preserved
	_, hasURL := props["url"]
	assert.True(t, hasURL, "original url field should be preserved in httpServerConfig.properties")
}

// TestFetchAndFixSchema_RegistryGuardPolicies_MissingSubStructures verifies that the
// registry/guard-policies injection is skipped gracefully when sub-structures are absent.
func TestFetchAndFixSchema_RegistryGuardPolicies_MissingSubStructures(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]interface{}
	}{
		{
			name: "stdioServerConfig has no properties",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"stdioServerConfig": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "httpServerConfig has no properties",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"httpServerConfig": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "neither stdioServerConfig nor httpServerConfig in definitions",
			schema: map[string]interface{}{
				"definitions": map[string]interface{}{
					"someOtherConfig": map[string]interface{}{"type": "object"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := schemaServer(t, tt.schema)
			defer srv.Close()

			result, err := fetchAndFixSchema(srv.URL)

			require.NoError(t, err, "missing sub-structures should not cause errors")
			assert.NotEmpty(t, result)
		})
	}
}

// TestFetchAndFixSchema_AllTransformationsApplied verifies that all three
// transformation branches run correctly when a single schema contains all the
// structures that trigger them.
func TestFetchAndFixSchema_AllTransformationsApplied(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		// Trigger #1: customServerConfig.type with negative-lookahead pattern
		"definitions": map[string]interface{}{
			"customServerConfig": map[string]interface{}{
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":    "string",
						"pattern": `^(?!stdio$|http$).*`,
					},
				},
			},
			"stdioServerConfig": map[string]interface{}{
				"properties": map[string]interface{}{
					"container": map[string]interface{}{"type": "string"},
				},
			},
			"httpServerConfig": map[string]interface{}{
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string"},
				},
			},
		},
		// Trigger #2: customSchemas.patternProperties with negative-lookahead key
		"properties": map[string]interface{}{
			"customSchemas": map[string]interface{}{
				"patternProperties": map[string]interface{}{
					`^(?!stdio$|http$)[a-z][a-z0-9-]*$`: map[string]interface{}{"type": "string"},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)

	// Verify transformation #1: customServerConfig.type
	defs := got["definitions"].(map[string]interface{})
	csConf := defs["customServerConfig"].(map[string]interface{})
	csProps := csConf["properties"].(map[string]interface{})
	typeField := csProps["type"].(map[string]interface{})
	_, hasPattern := typeField["pattern"]
	assert.False(t, hasPattern, "customServerConfig.type.pattern should be removed")
	notConstraint, hasNot := typeField["not"]
	assert.True(t, hasNot, "customServerConfig.type.not should be added")
	notEnum := notConstraint.(map[string]interface{})["enum"].([]interface{})
	assert.Contains(t, notEnum, "stdio")
	assert.Contains(t, notEnum, "http")

	// Verify transformation #2: patternProperties
	topProps := got["properties"].(map[string]interface{})
	patternProps := topProps["customSchemas"].(map[string]interface{})["patternProperties"].(map[string]interface{})
	_, hasNegLookahead := patternProps[`^(?!stdio$|http$)[a-z][a-z0-9-]*$`]
	assert.False(t, hasNegLookahead, "negative-lookahead key should be removed")
	_, hasSimple := patternProps["^[a-z][a-z0-9-]*$"]
	assert.True(t, hasSimple, "simple pattern key should be present")

	// Verify transformation #3: registry and guard-policies injected into both configs
	stdioConf := defs["stdioServerConfig"].(map[string]interface{})
	stdioProps := stdioConf["properties"].(map[string]interface{})
	_, hasStdioRegistry := stdioProps["registry"]
	_, hasStdioGP := stdioProps["guard-policies"]
	assert.True(t, hasStdioRegistry, "registry should be injected into stdioServerConfig")
	assert.True(t, hasStdioGP, "guard-policies should be injected into stdioServerConfig")

	httpConf := defs["httpServerConfig"].(map[string]interface{})
	httpProps := httpConf["properties"].(map[string]interface{})
	_, hasHTTPRegistry := httpProps["registry"]
	_, hasHTTPGP := httpProps["guard-policies"]
	assert.True(t, hasHTTPRegistry, "registry should be injected into httpServerConfig")
	assert.True(t, hasHTTPGP, "guard-policies should be injected into httpServerConfig")
}

// TestFetchAndFixSchema_PreservesExistingRegistryField verifies that if a schema
// already has a registry field in stdioServerConfig, the transformation overwrites it
// (last-write wins on map assignment).
func TestFetchAndFixSchema_PreservesSchemaIntegrity(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": map[string]interface{}{
			"stdioServerConfig": map[string]interface{}{
				"required": []string{"container"},
				"properties": map[string]interface{}{
					"container": map[string]interface{}{
						"type":        "string",
						"description": "Docker container image",
					},
					"args": map[string]interface{}{"type": "array"},
				},
			},
		},
	}
	srv := schemaServer(t, schema)
	defer srv.Close()

	result, err := fetchAndFixSchema(srv.URL)
	require.NoError(t, err)

	got := unmarshalSchema(t, result)
	defs := got["definitions"].(map[string]interface{})
	stdioConf := defs["stdioServerConfig"].(map[string]interface{})
	props := stdioConf["properties"].(map[string]interface{})

	// pre-existing fields must be preserved after transformation
	container, hasContainer := props["container"]
	require.True(t, hasContainer, "original container field should be preserved")
	containerMap := container.(map[string]interface{})
	assert.Equal(t, "Docker container image", containerMap["description"],
		"container description should be unchanged")

	_, hasArgs := props["args"]
	assert.True(t, hasArgs, "original args field should be preserved")

	// required array should also be preserved
	required, hasRequired := stdioConf["required"]
	assert.True(t, hasRequired, "required array should be preserved")
	requiredSlice := required.([]interface{})
	assert.Contains(t, requiredSlice, "container")
}
