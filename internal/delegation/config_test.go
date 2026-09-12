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

func TestRuntimeConfigControlDeps(t *testing.T) {
	t.Run("nil config yields disabled deps", func(t *testing.T) {
		var config *RuntimeConfig
		assert.Equal(t, ControlDeps{}, config.ControlDeps())
	})

	t.Run("forwards control fields", func(t *testing.T) {
		config := &RuntimeConfig{
			Store:             &Store{},
			Capability:        &ControlCapability{},
			StatePath:         "/tmp/delegation-state.json",
			ControlListenAddr: "127.0.0.1:9000",
		}
		assert.Equal(t, ControlDeps{
			Store:      config.Store,
			Capability: config.Capability,
			StatePath:  config.StatePath,
		}, config.ControlDeps())
	})
}
