package delegation

import "fmt"

// ControlPathPrefix is the private AWF control-plane URL prefix for
// github-repository-delegation-v1 operations.
const ControlPathPrefix = "/internal/awf-enclave-mcp-control/"

// RuntimeConfig enables runtime repository-read delegation and its
// AWF-authenticated private control channel.
type RuntimeConfig struct {
	Store             *Store
	Capability        *ControlCapability
	StatePath         string
	ControlListenAddr string
}

// Validate reports an error if a required runtime delegation field is missing.
func (c *RuntimeConfig) Validate() error {
	if c == nil || c.Store == nil || c.Capability == nil || c.StatePath == "" {
		return fmt.Errorf("delegation store, control capability, and state path are required")
	}
	return nil
}
