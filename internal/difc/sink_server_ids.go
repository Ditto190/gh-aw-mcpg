package difc

import (
	"sort"
	"strings"
	"sync"
)

var (
	sinkServerIDsMu sync.RWMutex
	sinkServerIDs   = []string{}
)

// SetSinkServerIDs configures backend server IDs that should receive DIFC tag snapshot
// enrichment in RPC JSONL logs.
func SetSinkServerIDs(serverIDs []string) {
	sinkServerIDsMu.Lock()
	defer sinkServerIDsMu.Unlock()

	if len(serverIDs) == 0 {
		sinkServerIDs = nil
		return
	}

	unique := make(map[string]struct{}, len(serverIDs))
	normalized := make([]string, 0, len(serverIDs))
	for _, serverID := range serverIDs {
		trimmed := strings.TrimSpace(serverID)
		if trimmed == "" {
			continue
		}
		if _, exists := unique[trimmed]; exists {
			continue
		}
		unique[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	sort.Strings(normalized)
	sinkServerIDs = normalized
}

// IsSinkServerID reports whether serverID is in the configured set of DIFC sink server IDs.
func IsSinkServerID(serverID string) bool {
	sinkServerIDsMu.RLock()
	defer sinkServerIDsMu.RUnlock()

	for _, sinkServerID := range sinkServerIDs {
		if serverID == sinkServerID {
			return true
		}
	}
	return false
}
