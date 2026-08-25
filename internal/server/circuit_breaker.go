package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/util"
)

// circuitBreakerState represents the state of a circuit breaker.
type circuitBreakerState int

const (
	// circuitClosed is normal operation — requests pass through.
	circuitClosed circuitBreakerState = iota
	// circuitOpen means the circuit is tripped — requests are rejected immediately.
	circuitOpen
	// circuitHalfOpen means one probe request is allowed to test recovery.
	circuitHalfOpen
)

func (s circuitBreakerState) String() string {
	switch s {
	case circuitClosed:
		return "CLOSED"
	case circuitOpen:
		return "OPEN"
	case circuitHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// DefaultRateLimitThreshold is the number of consecutive rate-limit errors
// before the circuit breaker opens.
const DefaultRateLimitThreshold = 3

// DefaultRateLimitCooldown is the number of seconds the circuit stays OPEN
// before transitioning to HALF-OPEN to probe one request.
const DefaultRateLimitCooldown = 60 * time.Second

// probeStrandedTimeout bounds how long a single HALF-OPEN probe may remain
// in flight. If the probe fails with a transport error (connection failure,
// JSON parse error, etc.) neither RecordSuccess nor RecordRateLimit is
// called, so without this timeout probeInFlight would never clear and the
// breaker would stay wedged in HALF-OPEN indefinitely, rejecting all
// subsequent requests. Once the timeout elapses, Allow re-releases a probe.
const probeStrandedTimeout = 30 * time.Second

var logCircuitBreaker = logger.ForFile()

// circuitBreaker implements a per-backend rate-limit circuit breaker.
//
// State transitions:
//
//	CLOSED  → OPEN      : after threshold consecutive rate-limit errors
//	OPEN    → HALF-OPEN : after cooldown period elapses
//	HALF-OPEN → CLOSED  : probe request succeeds
//	HALF-OPEN → OPEN    : probe request is rate-limited again
type circuitBreaker struct {
	mu sync.Mutex

	state             circuitBreakerState
	consecutiveErrors int
	openedAt          time.Time
	// resetAt is the time when the upstream rate limit resets, parsed from
	// the X-RateLimit-Reset header or the tool response message.
	resetAt       time.Time
	probeInFlight bool
	// probeStartedAt is when the current HALF-OPEN probe was allowed through.
	// Used to detect a stranded probe (e.g. one that failed with a transport
	// error rather than calling RecordSuccess/RecordRateLimit) so the breaker
	// doesn't stay wedged in HALF-OPEN forever.
	probeStartedAt time.Time
	serverID       string

	threshold int
	cooldown  time.Duration

	// nowFunc returns the current time. Defaults to time.Now; overridden in tests
	// to avoid flaky time.Sleep-based assertions.
	nowFunc func() time.Time
}

// newCircuitBreaker creates a circuit breaker for the given server ID.
// threshold is the number of consecutive rate-limit errors before opening;
// cooldown is how long to stay OPEN before probing.
func newCircuitBreaker(serverID string, threshold int, cooldown time.Duration) *circuitBreaker {
	if threshold <= 0 {
		threshold = DefaultRateLimitThreshold
	}
	if cooldown <= 0 {
		cooldown = DefaultRateLimitCooldown
	}
	logCircuitBreaker.Printf("Creating circuit breaker: serverID=%s, threshold=%d, cooldown=%v", serverID, threshold, cooldown)
	return &circuitBreaker{
		serverID:  serverID,
		state:     circuitClosed,
		threshold: threshold,
		cooldown:  cooldown,
		nowFunc:   time.Now,
	}
}

// ErrCircuitOpen is returned when the circuit breaker is OPEN (or HALF-OPEN
// with a probe already in flight) and a request is rejected.
type ErrCircuitOpen struct {
	ServerID string
	ResetAt  time.Time
	// State is the breaker's actual state at the time the error was created,
	// so callers/log messages don't misreport a HALF-OPEN breaker as OPEN.
	State circuitBreakerState
	// Now is the reference time used to compute the retry-after duration.
	// Defaults to time.Now when zero.
	Now time.Time
}

func (e *ErrCircuitOpen) Error() string {
	stateLabel := "OPEN"
	if e.State == circuitHalfOpen {
		stateLabel = "HALF-OPEN"
	}
	if e.ResetAt.IsZero() {
		return fmt.Sprintf("rate limit circuit breaker is %s for server %q — requests temporarily rejected", stateLabel, e.ServerID)
	}
	now := e.Now
	if now.IsZero() {
		now = time.Now()
	}
	retryAfter := e.ResetAt.Sub(now).Round(time.Second)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return fmt.Sprintf("rate limit circuit breaker is %s for server %q — rate limit resets at %s (retry after %s)",
		stateLabel, e.ServerID, e.ResetAt.UTC().Format(time.RFC3339), retryAfter)
}

// Allow reports whether a request should be allowed through. It also handles
// the OPEN → HALF-OPEN transition when the cooldown has elapsed.
// Returns an *ErrCircuitOpen error when the circuit is OPEN.
func (cb *circuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return nil

	case circuitOpen:
		// Check whether we should transition to HALF-OPEN.
		// We use the upstream reset time when available, otherwise the cooldown.
		now := cb.nowFunc()
		var openUntil time.Time
		if !cb.resetAt.IsZero() && cb.resetAt.After(cb.openedAt) {
			openUntil = cb.resetAt
		} else {
			openUntil = cb.openedAt.Add(cb.cooldown)
		}
		if now.After(openUntil) {
			logCircuitBreaker.Printf("server %q circuit breaker OPEN → HALF-OPEN after cooldown", cb.serverID)
			logger.LogInfo("backend", "circuit breaker for server %q transitioning OPEN → HALF-OPEN", cb.serverID)
			cb.state = circuitHalfOpen
			cb.probeInFlight = true
			cb.probeStartedAt = now
			return nil // allow the single probe
		}
		logCircuitBreaker.Printf("server %q circuit breaker OPEN, rejecting request (resetAt=%s)", cb.serverID, util.FormatFutureTime(cb.resetAt))
		return &ErrCircuitOpen{ServerID: cb.serverID, ResetAt: cb.resetAt, State: cb.state, Now: now}

	case circuitHalfOpen:
		now := cb.nowFunc()
		// Only one probe is allowed; further requests are blocked until the probe resolves.
		if cb.probeInFlight {
			// If the probe has been in flight longer than probeStrandedTimeout,
			// it likely failed with a transport error that never called
			// RecordSuccess/RecordRateLimit. Release a fresh probe rather than
			// staying wedged in HALF-OPEN forever.
			if !cb.probeStartedAt.IsZero() && now.Sub(cb.probeStartedAt) > probeStrandedTimeout {
				logCircuitBreaker.Printf("server %q circuit breaker HALF-OPEN probe stranded (in flight for %s) — releasing a new probe",
					cb.serverID, now.Sub(cb.probeStartedAt))
				logger.LogWarn("backend", "circuit breaker for server %q: stranded HALF-OPEN probe detected, releasing a new probe", cb.serverID)
				cb.probeStartedAt = now
				return nil
			}
			logCircuitBreaker.Printf("server %q circuit breaker HALF-OPEN, probe already in flight — rejecting request", cb.serverID)
			return &ErrCircuitOpen{ServerID: cb.serverID, ResetAt: cb.resetAt, State: cb.state, Now: now}
		}
		// This shouldn't normally happen (probe resolved but state wasn't updated),
		// but reserve a fresh probe defensively.
		cb.probeInFlight = true
		cb.probeStartedAt = now
		return nil
	}

	return nil
}

// RecordProbeReleased releases an in-flight HALF-OPEN probe slot without
// changing the breaker's state or consecutive-error count. It must be called
// when a request completes with a transport error (connection failure, JSON
// parse error, etc.) so the probe slot doesn't stay stranded until
// probeStrandedTimeout elapses — see the call site in unified.go for why
// transport errors must not affect rate-limit bookkeeping.
func (cb *circuitBreaker) RecordProbeReleased() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.probeInFlight {
		logCircuitBreaker.Printf("server %q circuit breaker releasing stranded HALF-OPEN probe after transport error", cb.serverID)
	}
	cb.probeInFlight = false
	cb.probeStartedAt = time.Time{}
}

// RecordSuccess records a successful (non-rate-limited) response.
// In HALF-OPEN state this closes the circuit.
func (cb *circuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	prev := cb.state
	if cb.consecutiveErrors > 0 {
		logCircuitBreaker.Printf("server %q circuit breaker resetting consecutive error count: %d → 0", cb.serverID, cb.consecutiveErrors)
	}
	cb.consecutiveErrors = 0
	cb.probeInFlight = false
	cb.probeStartedAt = time.Time{}
	if cb.state == circuitHalfOpen {
		cb.state = circuitClosed
		cb.resetAt = time.Time{}
		logCircuitBreaker.Printf("server %q circuit breaker HALF-OPEN → CLOSED (probe succeeded)", cb.serverID)
		logger.LogInfo("backend", "circuit breaker for server %q recovered: HALF-OPEN → CLOSED", cb.serverID)
	} else if prev != circuitClosed {
		cb.state = circuitClosed
	}
}

// RecordRateLimit records a rate-limit error for the given server.
// resetAt is the time the upstream rate limit resets (may be zero if unknown).
// When the consecutive error count reaches threshold the circuit opens.
func (cb *circuitBreaker) RecordRateLimit(resetAt time.Time) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveErrors++
	cb.probeInFlight = false
	cb.probeStartedAt = time.Time{}
	if !resetAt.IsZero() {
		cb.resetAt = resetAt
	}
	logCircuitBreaker.Printf("server %q recording rate-limit: consecutiveErrors=%d/%d, state=%s, hasResetAt=%v",
		cb.serverID, cb.consecutiveErrors, cb.threshold, cb.state, !cb.resetAt.IsZero())

	switch cb.state {
	case circuitClosed:
		if cb.consecutiveErrors >= cb.threshold {
			cb.state = circuitOpen
			cb.openedAt = cb.nowFunc()
			logger.LogError("backend",
				"circuit breaker for server %q OPENED after %d consecutive rate-limit errors; resets at %s",
				cb.serverID, cb.consecutiveErrors, util.FormatFutureTime(cb.resetAt))
			logCircuitBreaker.Printf("server %q circuit breaker CLOSED → OPEN (errors=%d)", cb.serverID, cb.consecutiveErrors)
		} else {
			logger.LogWarn("backend",
				"rate-limit error for server %q (consecutive=%d/%d); resets at %s",
				cb.serverID, cb.consecutiveErrors, cb.threshold, util.FormatFutureTime(cb.resetAt))
		}

	case circuitHalfOpen:
		// Probe failed — re-open the circuit.
		cb.state = circuitOpen
		cb.openedAt = cb.nowFunc()
		logger.LogError("backend",
			"circuit breaker for server %q re-OPENED after probe was rate-limited; resets at %s",
			cb.serverID, util.FormatFutureTime(cb.resetAt))
		logCircuitBreaker.Printf("server %q circuit breaker HALF-OPEN → OPEN (probe rate-limited)", cb.serverID)

	case circuitOpen:
		// Already open — update reset time.
		logCircuitBreaker.Printf("server %q recording rate-limit while already OPEN (consecutiveErrors=%d)", cb.serverID, cb.consecutiveErrors)
		logger.LogWarn("backend", "server %q circuit breaker still OPEN; resets at %s",
			cb.serverID, util.FormatFutureTime(cb.resetAt))
	}
}

// State returns the current circuit breaker state (for observability).
func (cb *circuitBreaker) State() circuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// buildCircuitBreakers creates per-backend circuit breakers from the configuration.
func buildCircuitBreakers(cfg *config.Config) map[string]*circuitBreaker {
	cbs := make(map[string]*circuitBreaker)
	if cfg == nil {
		return cbs
	}
	for serverID, serverCfg := range cfg.Servers {
		threshold := serverCfg.RateLimitThreshold
		cooldown := time.Duration(serverCfg.RateLimitCooldown) * time.Second
		cbs[serverID] = newCircuitBreaker(serverID, threshold, cooldown)
		logCircuitBreaker.Printf("Created circuit breaker for server %s: threshold=%d, cooldown=%s",
			serverID, threshold, cooldown)
	}
	logCircuitBreaker.Printf("buildCircuitBreakers: created %d circuit breakers", len(cbs))
	return cbs
}
