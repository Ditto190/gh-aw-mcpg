package difc

import (
	"sort"
	"time"
)

// ReflectedAgentLabels is the JSON shape for an agent's current DIFC labels.
type ReflectedAgentLabels struct {
	Secrecy   []string `json:"secrecy"`
	Integrity []string `json:"integrity"`
}

// ReflectResponse is the JSON response returned by /reflect endpoints.
type ReflectResponse struct {
	Agents    map[string]ReflectedAgentLabels `json:"agents"`
	Mode      string                          `json:"mode"`
	Timestamp string                          `json:"timestamp"`
}

// BuildReflectResponse returns a snapshot of all known agent labels.
func BuildReflectResponse(components DIFCComponents) ReflectResponse {
	agents := map[string]ReflectedAgentLabels{}
	if components.AgentRegistry != nil {
		for _, agentID := range components.AgentRegistry.GetAllAgentIDs() {
			agent, ok := components.AgentRegistry.Get(agentID)
			if !ok || agent == nil {
				continue
			}
			agents[agentID] = ReflectedAgentLabels{
				Secrecy:   tagsToStrings(agent.GetSecrecyTags()),
				Integrity: tagsToStrings(agent.GetIntegrityTags()),
			}
		}
	}

	return ReflectResponse{
		Agents:    agents,
		Mode:      components.Mode.String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func tagsToStrings(tags []Tag) []string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		out[i] = string(tag)
	}
	sort.Strings(out)
	return out
}
