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
		wantErr string
	}{
		{name: "nil config", config: nil, wantErr: "delegation runtime config is required"},
		{name: "missing store", config: &RuntimeConfig{Capability: valid.Capability, StatePath: valid.StatePath}, wantErr: "delegation store is required"},
		{name: "missing capability", config: &RuntimeConfig{Store: valid.Store, StatePath: valid.StatePath}, wantErr: "delegation control capability is required"},
		{name: "missing state path", config: &RuntimeConfig{Store: valid.Store, Capability: valid.Capability}, wantErr: "delegation state path is required"},
		{name: "valid", config: valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
