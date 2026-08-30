package server

import (
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/util"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// agentPoliciesEnforced reports whether per-agent access policies are configured.
// When false, per-agent enforcement is disabled and all agents retain full access
// (backward compatible with configurations that predate per-agent policies).
func (us *UnifiedServer) agentPoliciesEnforced() bool {
	return us.cfg != nil && us.cfg.AgentPoliciesEnabled()
}

// isMultiAgent reports whether more than one agent identity is configured. In that
// case the gateway is a shared resource and destructive, gateway-wide operations
// requested by a single principal (e.g. /close) must be refused so one agent cannot
// disrupt another. Nil-safe: returns false when there is no config (singular mode).
func (us *UnifiedServer) isMultiAgent() bool {
	if us == nil || us.cfg == nil {
		return false
	}
	return len(us.cfg.GetAgentIDs()) > 1
}

// agentCanAccessServer reports whether the given agent may access the named MCP
// server. Fail-closed: when policies are enforced and the agent has no policy (or
// the server is not permitted by it), access is denied. When policies are not
// enforced, all access is allowed.
func (us *UnifiedServer) agentCanAccessServer(agentID, serverID string) bool {
	if !us.agentPoliciesEnforced() {
		return true
	}
	policy := us.cfg.AgentPolicyFor(agentID)
	if policy == nil {
		logUnified.Printf("agentCanAccessServer: no policy for agent; denying serverID=%s", serverID)
		return false
	}
	return policy.AllowsServer(serverID)
}

// agentCanUseTool reports whether the given agent may call toolName on serverID.
// Fail-closed with the same semantics as agentCanAccessServer.
func (us *UnifiedServer) agentCanUseTool(agentID, serverID, toolName string) bool {
	if !us.agentPoliciesEnforced() {
		return true
	}
	policy := us.cfg.AgentPolicyFor(agentID)
	if policy == nil {
		logUnified.Printf("agentCanUseTool: no policy for agent; denying serverID=%s tool=%s", serverID, toolName)
		return false
	}
	return policy.AllowsTool(serverID, toolName)
}

// createAgentFilteredUnifiedServer builds an SDK server that exposes only the
// prefixed tools the given agent is permitted to use, reusing the unified server's
// handlers. It is used in unified mode when per-agent policies are in effect so
// that tools/list and tools/call reflect the agent's policy.
func createAgentFilteredUnifiedServer(us *UnifiedServer, agentID string) *sdk.Server {
	server := newSDKServer("awmg-unified", logTransport)

	us.toolsMu.RLock()
	tools := make([]*ToolInfo, 0, len(us.tools))
	for _, t := range us.tools {
		tools = append(tools, t)
	}
	us.toolsMu.RUnlock()

	registered := 0
	for _, toolInfo := range tools {
		backendID := toolInfo.BackendID
		unprefixed := strings.TrimPrefix(toolInfo.Name, backendID+"___")
		if !us.agentCanUseTool(agentID, backendID, unprefixed) {
			continue
		}
		if toolInfo.Handler == nil {
			continue
		}
		registerToolWithoutValidation(server, &sdk.Tool{
			Name:        toolInfo.Name,
			Description: toolInfo.Description,
			InputSchema: toolInfo.InputSchema,
			Annotations: toolInfo.Annotations,
		}, toolInfo.Handler)
		registered++
	}

	logger.LogInfo("client", "Built per-agent unified tool view: agent=%s, tools=%d",
		util.HashIdentifierForLog(agentID), registered)
	return server
}
