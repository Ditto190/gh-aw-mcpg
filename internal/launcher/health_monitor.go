package launcher

import (
	"log"
	"time"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

const (
	// DefaultHealthCheckInterval is the recommended periodic health check interval (spec §8).
	DefaultHealthCheckInterval = 30 * time.Second

	// maxConsecutiveRestartFailures caps how many consecutive restart failures
	// are allowed before the monitor stops retrying a particular server.
	maxConsecutiveRestartFailures = 3
)

var logHealth = logger.New("launcher:health")

// HealthMonitor periodically checks backend server health and automatically
// restarts servers that are in an error state (MCP Gateway Specification §8).
type HealthMonitor struct {
	launcher *Launcher
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}

	// Track consecutive restart failures per server to avoid infinite retry loops.
	consecutiveFailures map[string]int
}

// NewHealthMonitor creates a health monitor for the given launcher.
func NewHealthMonitor(l *Launcher, interval time.Duration) *HealthMonitor {
	if interval <= 0 {
		interval = DefaultHealthCheckInterval
	}
	return &HealthMonitor{
		launcher:            l,
		interval:            interval,
		stopCh:              make(chan struct{}),
		doneCh:              make(chan struct{}),
		consecutiveFailures: make(map[string]int),
	}
}

// Start begins periodic health checks in a background goroutine.
func (hm *HealthMonitor) Start() {
	log.Printf("[HEALTH] Starting health monitor (interval=%s)", hm.interval)
	logger.LogInfo("startup", "Health monitor started (interval=%s)", hm.interval)
	go hm.run()
}

// Stop signals the health monitor to stop and waits for it to finish.
func (hm *HealthMonitor) Stop() {
	close(hm.stopCh)
	<-hm.doneCh
	logHealth.Print("Health monitor stopped")
	logger.LogInfo("shutdown", "Health monitor stopped")
}

func (hm *HealthMonitor) run() {
	defer close(hm.doneCh)

	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.stopCh:
			return
		case <-hm.launcher.ctx.Done():
			return
		case <-ticker.C:
			hm.checkAll()
		}
	}
}

// checkAll iterates over every configured backend and attempts to restart
// any server that is in an error state.
func (hm *HealthMonitor) checkAll() {
	for _, serverID := range hm.launcher.ServerIDs() {
		state := hm.launcher.GetServerState(serverID)

		switch state.Status {
		case "error":
			hm.handleErrorState(serverID, state)
		case "running":
			// Reset consecutive failure counter on healthy server.
			if hm.consecutiveFailures[serverID] > 0 {
				hm.consecutiveFailures[serverID] = 0
			}
		}
	}
}

func (hm *HealthMonitor) handleErrorState(serverID string, state ServerState) {
	failures := hm.consecutiveFailures[serverID]
	if failures >= maxConsecutiveRestartFailures {
		// Already logged when the threshold was reached; stay silent.
		return
	}

	logger.LogWarn("backend", "Health check: server %q in error state (%s), attempting restart (%d/%d)",
		serverID, state.LastError, failures+1, maxConsecutiveRestartFailures)

	// Clear error state and cached connection so GetOrLaunch can retry.
	hm.launcher.clearServerForRestart(serverID)

	_, err := GetOrLaunch(hm.launcher, serverID)
	if err != nil {
		hm.consecutiveFailures[serverID] = failures + 1
		logger.LogError("backend", "Health check: restart failed for server %q: %v (attempt %d/%d)",
			serverID, err, failures+1, maxConsecutiveRestartFailures)
		if hm.consecutiveFailures[serverID] >= maxConsecutiveRestartFailures {
			logger.LogError("backend",
				"Health check: server %q reached max restart attempts (%d), will not retry until gateway restart",
				serverID, maxConsecutiveRestartFailures)
		}
		return
	}

	hm.consecutiveFailures[serverID] = 0
	log.Printf("[HEALTH] Successfully restarted server %q", serverID)
	logger.LogInfo("backend", "Health check: successfully restarted server %q", serverID)
}
