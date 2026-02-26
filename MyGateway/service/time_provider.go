package service

import (
	"mygateway/helpers"
	"mygateway/interfaces"
	"time"
)

// timeProvider implements interfaces.TimeProvider. It returns the current time via the injected now func.
// Used by service.JWTValidator for token expiry checks and by tests for deterministic time. Built in cmd/main with time.Now().UTC.
type timeProvider struct {
	now func() time.Time
}

// NewTimeProvider creates a TimeProvider that returns time via the given now func. Panics on nil now.
//
// Parameter now — no-arg function returning current time (in prod — time.Now().UTC, in tests — fixed time).
//
// Returns: interfaces.TimeProvider (*timeProvider).
//
// Called from cmd/main when building the gateway.
func NewTimeProvider(now func() time.Time) interfaces.TimeProvider {
	return &timeProvider{now: helpers.NilPanic(now, "service.time_provider.go: now is required")}
}

// Now returns current time from the injected function (UTC in prod or fixed in tests).
//
// Returns: time.Time.
//
// Called from service.jwtValidator.ValidateToken when checking token expires_at.
func (t *timeProvider) Now() time.Time {
	return t.now()
}
