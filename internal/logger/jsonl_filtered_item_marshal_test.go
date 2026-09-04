package logger

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONLFilteredItem_MarshalJSON exercises all branches of
// JSONLFilteredItem.MarshalJSON:
//   - AgentLabelsComplete=false: agent_secrecy_tags/agent_integrity_tags are
//     omitted entirely when nil/empty (via the `omitempty` json tag), and
//     preserved as-is when non-empty.
//   - AgentLabelsComplete=true: agent_secrecy_tags/agent_integrity_tags are
//     always present in the output, even when nil/empty, encoding them as
//     explicit empty arrays ("[]") rather than omitting the fields.
func TestJSONLFilteredItem_MarshalJSON(t *testing.T) {
	tests := []struct {
		name                    string
		entry                   JSONLFilteredItem
		wantSecrecyPresent      bool
		wantSecrecyValue        []string
		wantIntegrityPresent    bool
		wantIntegrityValue      []string
		wantOtherFieldsRoundtrp bool
	}{
		{
			name: "incomplete labels with nil tags omits both fields",
			entry: JSONLFilteredItem{
				Timestamp: "2024-01-01T00:00:00.000Z",
				Event:     "difc_filtered",
				Schema:    "difc-filtered/v2",
				FilteredItemLogEntry: FilteredItemLogEntry{
					ServerID:            "github",
					ToolName:            "create_issue",
					AgentLabelsComplete: false,
				},
			},
			wantSecrecyPresent:   false,
			wantIntegrityPresent: false,
		},
		{
			name: "incomplete labels with populated tags preserves them",
			entry: JSONLFilteredItem{
				Timestamp: "2024-01-01T00:00:00.000Z",
				Event:     "difc_filtered",
				Schema:    "difc-filtered/v2",
				FilteredItemLogEntry: FilteredItemLogEntry{
					ServerID:            "github",
					ToolName:            "create_issue",
					AgentLabelsComplete: false,
					AgentSecrecyTags:    []string{"private:org/repo"},
					AgentIntegrityTags:  []string{"approved"},
				},
			},
			wantSecrecyPresent:   true,
			wantSecrecyValue:     []string{"private:org/repo"},
			wantIntegrityPresent: true,
			wantIntegrityValue:   []string{"approved"},
		},
		{
			name: "complete labels with nil tags forces explicit empty arrays",
			entry: JSONLFilteredItem{
				Timestamp: "2024-01-01T00:00:00.000Z",
				Event:     "difc_filtered",
				Schema:    "difc-filtered/v2",
				FilteredItemLogEntry: FilteredItemLogEntry{
					ServerID:            "github",
					ToolName:            "create_issue",
					AgentLabelsComplete: true,
					AgentSecrecyTags:    nil,
					AgentIntegrityTags:  nil,
				},
			},
			wantSecrecyPresent:   true,
			wantSecrecyValue:     nil,
			wantIntegrityPresent: true,
			wantIntegrityValue:   nil,
		},
		{
			name: "complete labels with populated tags keeps them present",
			entry: JSONLFilteredItem{
				Timestamp: "2024-01-01T00:00:00.000Z",
				Event:     "difc_filtered",
				Schema:    "difc-filtered/v2",
				FilteredItemLogEntry: FilteredItemLogEntry{
					ServerID:            "github",
					ToolName:            "create_issue",
					AgentLabelsComplete: true,
					AgentSecrecyTags:    []string{"public"},
					AgentIntegrityTags:  []string{"merged", "approved"},
				},
			},
			wantSecrecyPresent:   true,
			wantSecrecyValue:     []string{"public"},
			wantIntegrityPresent: true,
			wantIntegrityValue:   []string{"merged", "approved"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.entry.MarshalJSON()
			require.NoError(t, err, "MarshalJSON must not error")

			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &fields), "output must be valid JSON")

			secrecyRaw, secrecyPresent := fields["agent_secrecy_tags"]
			assert.Equal(t, tt.wantSecrecyPresent, secrecyPresent, "agent_secrecy_tags presence mismatch")
			if tt.wantSecrecyPresent {
				var got []string
				require.NoError(t, json.Unmarshal(secrecyRaw, &got))
				assert.Equal(t, tt.wantSecrecyValue, got)
			}

			integrityRaw, integrityPresent := fields["agent_integrity_tags"]
			assert.Equal(t, tt.wantIntegrityPresent, integrityPresent, "agent_integrity_tags presence mismatch")
			if tt.wantIntegrityPresent {
				var got []string
				require.NoError(t, json.Unmarshal(integrityRaw, &got))
				assert.Equal(t, tt.wantIntegrityValue, got)
			}

			// Other fields must always round-trip correctly regardless of the
			// AgentLabelsComplete branch taken.
			var serverID, toolName string
			require.NoError(t, json.Unmarshal(fields["server_id"], &serverID))
			require.NoError(t, json.Unmarshal(fields["tool_name"], &toolName))
			assert.Equal(t, tt.entry.ServerID, serverID)
			assert.Equal(t, tt.entry.ToolName, toolName)
		})
	}
}

// TestJSONLFilteredItem_MarshalJSON_ViaEncodingJSON verifies that the custom
// MarshalJSON is correctly invoked via the standard json.Marshal entry point
// (not just when called directly), including through a pointer value and as
// part of an enclosing struct/slice.
func TestJSONLFilteredItem_MarshalJSON_ViaEncodingJSON(t *testing.T) {
	item := JSONLFilteredItem{
		Timestamp: "2024-01-01T00:00:00.000Z",
		Event:     "difc_filtered",
		Schema:    "difc-filtered/v2",
		FilteredItemLogEntry: FilteredItemLogEntry{
			ServerID:            "slack",
			ToolName:            "send_message",
			AgentLabelsComplete: true,
		},
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	_, hasSecrecy := fields["agent_secrecy_tags"]
	_, hasIntegrity := fields["agent_integrity_tags"]
	assert.True(t, hasSecrecy, "agent_secrecy_tags must be present when AgentLabelsComplete is true")
	assert.True(t, hasIntegrity, "agent_integrity_tags must be present when AgentLabelsComplete is true")

	// Marshal via pointer too, since encoding/json dispatches differently
	// for pointer vs value receivers in some contexts (slices of pointers).
	items := []*JSONLFilteredItem{&item}
	dataSlice, err := json.Marshal(items)
	require.NoError(t, err)
	assert.Contains(t, string(dataSlice), "agent_secrecy_tags")
}
