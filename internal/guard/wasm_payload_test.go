package guard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBoolFailure(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]interface{}
		resultJSON []byte
		key        string
		wantErr    string
	}{
		{
			name:       "key absent - no failure",
			raw:        map[string]interface{}{},
			resultJSON: []byte(`{}`),
			key:        "success",
			wantErr:    "",
		},
		{
			name:       "key true - no failure",
			raw:        map[string]interface{}{"success": true},
			resultJSON: []byte(`{"success":true}`),
			key:        "success",
			wantErr:    "",
		},
		{
			name:       "key false with error message",
			raw:        map[string]interface{}{"success": false, "error": "policy rejected"},
			resultJSON: []byte(`{"success":false,"error":"policy rejected"}`),
			key:        "success",
			wantErr:    "label_agent rejected policy: policy rejected",
		},
		{
			name:       "key false without error message",
			raw:        map[string]interface{}{"success": false},
			resultJSON: []byte(`{"success":false}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key false with empty error message",
			raw:        map[string]interface{}{"success": false, "error": ""},
			resultJSON: []byte(`{"success":false,"error":""}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key false with whitespace error message",
			raw:        map[string]interface{}{"success": false, "error": "   "},
			resultJSON: []byte(`{"success":false,"error":"   "}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key is non-bool value - treated as absent",
			raw:        map[string]interface{}{"success": "true"},
			resultJSON: []byte(`{"success":"true"}`),
			key:        "success",
			wantErr:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkBoolFailure(tt.raw, tt.resultJSON, tt.key)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
