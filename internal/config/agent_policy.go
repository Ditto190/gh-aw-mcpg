package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/util"
)

// AgentPolicy defines the per-agent access policy for a single authenticated
// agent identity. It restricts which MCP servers and tools the agent may use and
// may carry an optional per-agent allow-only guard policy that is applied to the
// agent's DIFC guard session.
//
// The policy is fail-closed: when per-agent policies are configured, an agent may
// only access servers explicitly listed in Servers and, when Tools restricts a
// server, only the listed tools on that server.
type AgentPolicy struct {
	// Servers lists the MCP server IDs this agent may access. An agent may only
	// reach servers listed here. Each entry must reference a configured server.
	Servers []string `toml:"servers" json:"servers,omitempty"`

	// Tools optionally restricts, per server, the tool names the agent may call.
	// A server present in Servers but absent from Tools means all tools on that
	// server are permitted. A list containing "*" permits all tools on that server.
	// Every key must also appear in Servers.
	Tools map[string][]string `toml:"tools" json:"tools,omitempty"`

	// AllowOnly is an optional per-agent allow-only guard policy applied to this
	// agent's DIFC guard session. Enforcement requires an active (non-noop) guard.
	AllowOnly *AllowOnlyPolicy `toml:"allow-only" json:"allow-only,omitempty"`
}

// AllowsServer reports whether the agent may access the named MCP server.
func (p *AgentPolicy) AllowsServer(serverID string) bool {
	if p == nil {
		return false
	}
	for _, s := range p.Servers {
		if s == serverID {
			return true
		}
	}
	return false
}

// AllowsTool reports whether the agent may call toolName on serverID. The server
// must be permitted; when a per-server tool allowlist is configured, the tool must
// appear in it (or the list must contain the "*" wildcard). When no per-server
// allowlist is configured for a permitted server, all of its tools are allowed.
func (p *AgentPolicy) AllowsTool(serverID, toolName string) bool {
	if !p.AllowsServer(serverID) {
		logValidation.Printf("AllowsTool: denying tool=%s on server=%s (server not permitted)", toolName, serverID)
		return false
	}
	tools, ok := p.Tools[serverID]
	if !ok {
		return true
	}
	for _, t := range tools {
		if t == "*" || t == toolName {
			return true
		}
	}
	logValidation.Printf("AllowsTool: denying tool=%s on server=%s (not in per-server allowlist)", toolName, serverID)
	return false
}

// AgentPoliciesEnabled reports whether any per-agent access policies are configured.
// When false, per-agent enforcement is disabled and all agents retain full access
// (backward compatible with configurations that predate per-agent policies).
func (c *Config) AgentPoliciesEnabled() bool {
	return c != nil && c.Gateway != nil && len(c.Gateway.AgentPolicies) > 0
}

// AgentPolicyFor returns the configured policy for the given agent ID, or nil when
// no policy is configured for that identity.
func (c *Config) AgentPolicyFor(agentID string) *AgentPolicy {
	if c == nil || c.Gateway == nil {
		return nil
	}
	policy, ok := c.Gateway.AgentPolicies[agentID]
	if !ok {
		logValidation.Printf("AgentPolicyFor: no policy configured for agentID=%s", util.HashIdentifierForLog(agentID))
	}
	return policy
}

// HasAgentAllowOnlyPolicies reports whether any configured per-agent policy carries
// an allow-only guard policy. Used to auto-enable DIFC enforcement.
func (c *Config) HasAgentAllowOnlyPolicies() bool {
	if c == nil || c.Gateway == nil {
		return false
	}
	for _, p := range c.Gateway.AgentPolicies {
		if p != nil && p.AllowOnly != nil {
			return true
		}
	}
	return false
}

// validateAgentPolicies validates the per-agent policy map against the configured
// agent IDs and servers. It enforces the fail-closed model:
//
//   - Unknown policy keys (not a configured agent ID) are rejected.
//   - In multi-agent mode (more than one configured agent ID) with policies present,
//     every configured agent ID must have a policy (deterministic fail-closed startup).
//   - Referenced servers must exist; per-server tool keys must be within the policy's
//     servers; duplicate server references and duplicate tool entries are rejected.
//   - Any per-agent allow-only policy is validated with the guard policy validator.
//
// When no policies are configured, singular configurations retain their legacy
// full-access behavior. Multi-agent configurations fail closed because an
// unscoped additional identity cannot safely share the gateway.
func validateAgentPolicies(cfg *Config) error {
	if cfg == nil || cfg.Gateway == nil {
		return nil
	}
	policies := cfg.Gateway.AgentPolicies
	if len(policies) == 0 {
		if len(cfg.GetAgentIDs()) > 1 {
			return fmt.Errorf("gateway.agent_policies must define a policy for every configured agent ID when multiple agent IDs are set (fail-closed)")
		}
		logValidation.Print("No per-agent policies configured; skipping validation")
		return nil
	}

	agentIDs := cfg.GetAgentIDs()
	agentIDSet := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		agentIDSet[id] = struct{}{}
	}
	logValidation.Printf("Validating %d per-agent policies against %d configured agent IDs", len(policies), len(agentIDs))

	// Reject policies keyed by an unknown (unconfigured) agent ID.
	for policyID := range policies {
		if _, ok := agentIDSet[policyID]; !ok {
			return fmt.Errorf("gateway.agent_policies references unknown agent ID %q; it must match a configured agent_id/agent_ids entry", util.HashIdentifierForLog(policyID))
		}
	}

	// Fail-closed: in multi-agent mode, every configured agent must have a policy.
	if len(agentIDs) > 1 {
		var missing []string
		for _, id := range agentIDs {
			if _, ok := policies[id]; !ok {
				missing = append(missing, util.HashIdentifierForLog(id))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("gateway.agent_policies must define a policy for every configured agent ID when multiple agent IDs are set (fail-closed); missing: %s", strings.Join(missing, ", "))
		}
	}

	for policyID, policy := range policies {
		if policy == nil {
			return fmt.Errorf("gateway.agent_policies[%q] must not be null", util.HashIdentifierForLog(policyID))
		}
		if err := validateSingleAgentPolicy(policyID, policy, cfg.Servers); err != nil {
			return err
		}
	}

	return nil
}

// validateSingleAgentPolicy validates one agent's policy: server references, the
// per-server tool allowlist, and any allow-only guard policy.
func validateSingleAgentPolicy(policyID string, policy *AgentPolicy, servers map[string]*ServerConfig) error {
	formattedPolicyID := util.HashIdentifierForLog(policyID)
	logValidation.Printf("Validating agent policy for %s: %d servers, %d per-server tool allowlists, allowOnly=%v",
		formattedPolicyID, len(policy.Servers), len(policy.Tools), policy.AllowOnly != nil)
	serverSet := make(map[string]struct{}, len(policy.Servers))
	serverIDs := make([]string, 0, len(policy.Servers))
	for _, serverID := range policy.Servers {
		trimmed := strings.TrimSpace(serverID)
		if trimmed == "" {
			return fmt.Errorf("gateway.agent_policies[%q].servers entries must be non-empty strings", formattedPolicyID)
		}
		if trimmed != serverID {
			return fmt.Errorf("gateway.agent_policies[%q].servers entries must not contain surrounding whitespace", formattedPolicyID)
		}
		if _, ok := servers[trimmed]; !ok {
			return fmt.Errorf("gateway.agent_policies[%q].servers references unknown server %q", formattedPolicyID, trimmed)
		}
		serverSet[trimmed] = struct{}{}
		serverIDs = append(serverIDs, trimmed)
	}
	if duplicate, found := util.FindDuplicate(serverIDs); found {
		return fmt.Errorf("gateway.agent_policies[%q].servers must not contain duplicate server %q", formattedPolicyID, duplicate)
	}

	for serverID, tools := range policy.Tools {
		if strings.TrimSpace(serverID) != serverID {
			return fmt.Errorf("gateway.agent_policies[%q].tools server keys must not contain surrounding whitespace", formattedPolicyID)
		}
		if _, ok := serverSet[serverID]; !ok {
			return fmt.Errorf("gateway.agent_policies[%q].tools references server %q that is not in the policy's servers list", formattedPolicyID, serverID)
		}
		toolNames := make([]string, 0, len(tools))
		for _, toolName := range tools {
			trimmed := strings.TrimSpace(toolName)
			if trimmed == "" {
				return fmt.Errorf("gateway.agent_policies[%q].tools[%q] entries must be non-empty strings", formattedPolicyID, serverID)
			}
			if trimmed != toolName {
				return fmt.Errorf("gateway.agent_policies[%q].tools[%q] entries must not contain surrounding whitespace", formattedPolicyID, serverID)
			}
			toolNames = append(toolNames, trimmed)
		}
		if duplicate, found := util.FindDuplicate(toolNames); found {
			return fmt.Errorf("gateway.agent_policies[%q].tools[%q] must not contain duplicate tool %q", formattedPolicyID, serverID, duplicate)
		}
	}

	if policy.AllowOnly != nil {
		if err := ValidateGuardPolicy(&GuardPolicy{AllowOnly: policy.AllowOnly}); err != nil {
			return fmt.Errorf("gateway.agent_policies[%q].allow-only is invalid: %w", formattedPolicyID, err)
		}
	}

	return nil
}
