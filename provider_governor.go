package ragagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	providerCircuitStateClosed   = "closed"
	providerCircuitStateOpen     = "open"
	providerCircuitStateHalfOpen = "half_open"
)

type providerGovernors struct {
	cfg    ProviderGovernanceConfig
	logger Logger

	mu        sync.Mutex
	providers map[string]*providerGovernor
}

func newProviderGovernors(cfg ProviderGovernanceConfig, logger Logger) *providerGovernors {
	return &providerGovernors{
		cfg:       cfg.normalized(),
		logger:    logger,
		providers: make(map[string]*providerGovernor),
	}
}

func (g *providerGovernors) forProvider(provider string) *providerGovernor {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if governor, ok := g.providers[provider]; ok {
		return governor
	}
	governor := newProviderGovernor(provider, g.cfg, g.logger)
	g.providers[provider] = governor
	return governor
}

type providerGovernor struct {
	rateLimiter    *providerRateLimiter
	circuitBreaker *providerCircuitBreaker
}

func newProviderGovernor(provider string, cfg ProviderGovernanceConfig, logger Logger) *providerGovernor {
	cfg = cfg.normalized()
	return &providerGovernor{
		rateLimiter:    newProviderRateLimiter(cfg.RateLimit),
		circuitBreaker: newProviderCircuitBreaker(provider, cfg.CircuitBreaker, logger),
	}
}

type providerAttemptHandle struct {
	ThrottleDelay time.Duration
	CircuitState  string
	reservation   *providerCircuitReservation
}

func (g *providerGovernor) acquire(ctx context.Context) (providerAttemptHandle, error) {
	handle := providerAttemptHandle{
		CircuitState: providerCircuitStateClosed,
	}
	if g == nil {
		return handle, nil
	}
	if g.circuitBreaker != nil {
		reservation, state, err := g.circuitBreaker.reserve(time.Now())
		handle.CircuitState = state
		if err != nil {
			return handle, err
		}
		handle.reservation = reservation
	}
	if g.rateLimiter != nil {
		delay, err := g.rateLimiter.wait(ctx)
		handle.ThrottleDelay = delay
		if err != nil {
			handle.abort()
			return handle, err
		}
	}
	return handle, nil
}

func (h providerAttemptHandle) finish(class string) {
	if h.reservation == nil {
		return
	}
	h.reservation.finish(class)
}

func (h providerAttemptHandle) abort() {
	if h.reservation == nil {
		return
	}
	h.reservation.abort()
}

type providerRateLimiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newProviderRateLimiter(cfg ProviderRateLimitConfig) *providerRateLimiter {
	if !cfg.enabled() {
		return nil
	}
	now := time.Now()
	return &providerRateLimiter{
		rate:   cfg.RequestsPerSecond,
		burst:  float64(cfg.Burst),
		tokens: float64(cfg.Burst),
		last:   now,
	}
}

func (l *providerRateLimiter) wait(ctx context.Context) (time.Duration, error) {
	if l == nil {
		return 0, nil
	}
	started := time.Now()
	for {
		wait := l.take()
		if wait <= 0 {
			return time.Since(started), nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return time.Since(started), ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *providerRateLimiter) take() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	if elapsed > 0 {
		l.tokens = min(l.burst, l.tokens+elapsed*l.rate)
		l.last = now
	}
	if l.tokens >= 1 {
		l.tokens--
		return 0
	}
	missing := 1 - l.tokens
	if missing <= 0 {
		return 0
	}
	return time.Duration((missing / l.rate) * float64(time.Second))
}

type providerCircuitBreaker struct {
	provider string
	cfg      ProviderCircuitBreakerConfig
	logger   Logger

	mu                 sync.Mutex
	state              string
	consecutiveFailure int
	openedAt           time.Time
	halfOpenInFlight   int
}

func newProviderCircuitBreaker(provider string, cfg ProviderCircuitBreakerConfig, logger Logger) *providerCircuitBreaker {
	if !cfg.enabled() {
		return nil
	}
	return &providerCircuitBreaker{
		provider: provider,
		cfg:      cfg.normalized(),
		logger:   logger,
		state:    providerCircuitStateClosed,
	}
}

type providerCircuitReservation struct {
	breaker  *providerCircuitBreaker
	halfOpen bool
}

func (b *providerCircuitBreaker) reserve(now time.Time) (*providerCircuitReservation, string, error) {
	if b == nil {
		return nil, providerCircuitStateClosed, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case providerCircuitStateOpen:
		if now.Sub(b.openedAt) < b.cfg.OpenTimeout {
			return nil, providerCircuitStateOpen, fmt.Errorf("%w: provider=%s", ErrProviderCircuitOpen, b.provider)
		}
		b.transitionLocked(providerCircuitStateHalfOpen)
	case providerCircuitStateHalfOpen:
		if b.halfOpenInFlight >= b.cfg.HalfOpenMaxCalls {
			return nil, providerCircuitStateHalfOpen, fmt.Errorf("%w: provider=%s", ErrProviderCircuitOpen, b.provider)
		}
	case providerCircuitStateClosed:
	default:
		b.state = providerCircuitStateClosed
	}

	reservation := &providerCircuitReservation{breaker: b}
	if b.state == providerCircuitStateHalfOpen {
		b.halfOpenInFlight++
		reservation.halfOpen = true
	}
	return reservation, b.state, nil
}

func (r *providerCircuitReservation) abort() {
	if r == nil || r.breaker == nil {
		return
	}
	r.breaker.mu.Lock()
	defer r.breaker.mu.Unlock()
	if r.halfOpen && r.breaker.halfOpenInFlight > 0 {
		r.breaker.halfOpenInFlight--
	}
}

func (r *providerCircuitReservation) finish(class string) {
	if r == nil || r.breaker == nil {
		return
	}
	r.breaker.mu.Lock()
	defer r.breaker.mu.Unlock()

	if r.halfOpen && r.breaker.halfOpenInFlight > 0 {
		r.breaker.halfOpenInFlight--
	}

	if class == "canceled" {
		return
	}

	switch r.breaker.state {
	case providerCircuitStateClosed:
		if isCircuitFailureClass(class) {
			r.breaker.consecutiveFailure++
			if r.breaker.consecutiveFailure >= r.breaker.cfg.FailureThreshold {
				r.breaker.consecutiveFailure = 0
				r.breaker.openedAt = time.Now()
				r.breaker.transitionLocked(providerCircuitStateOpen)
			}
			return
		}
		r.breaker.consecutiveFailure = 0
	case providerCircuitStateHalfOpen:
		if isCircuitFailureClass(class) {
			r.breaker.openedAt = time.Now()
			r.breaker.transitionLocked(providerCircuitStateOpen)
			return
		}
		r.breaker.consecutiveFailure = 0
		r.breaker.transitionLocked(providerCircuitStateClosed)
	}
}

func (b *providerCircuitBreaker) transitionLocked(next string) {
	if b.state == next {
		return
	}
	prev := b.state
	b.state = next
	if b.logger == nil {
		return
	}
	b.logger.Info("ragagent provider circuit state changed",
		"provider", b.provider,
		"from", prev,
		"to", next,
		"failure_threshold", b.cfg.FailureThreshold,
		"open_timeout", b.cfg.OpenTimeout,
	)
}

func isCircuitFailureClass(class string) bool {
	switch class {
	case providerErrorClassTransient, providerErrorClassRateLimit:
		return true
	default:
		return false
	}
}
