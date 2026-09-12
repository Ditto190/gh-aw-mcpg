package delegation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeConfigValidate(t *testing.T) {
	valid := &RuntimeConfig{
		Store:      &Store{},
		Capability: &ControlCapability{},
		StatePath:  "/tmp/delegation-state.json",
	}
	tests := []struct {
		name    string
		config  *RuntimeConfig
		wantErr bool
	}{
		{name: "nil config", config: nil, wantErr: true},
		{name: "missing store", config: &RuntimeConfig{Capability: valid.Capability, StatePath: valid.StatePath}, wantErr: true},
		{name: "missing capability", config: &RuntimeConfig{Store: valid.Store, StatePath: valid.StatePath}, wantErr: true},
		{name: "missing state path", config: &RuntimeConfig{Store: valid.Store, Capability: valid.Capability}, wantErr: true},
		{name: "valid", config: valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.EqualError(t, err, "delegation store, control capability, and state path are required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
