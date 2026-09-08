package delegation

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
