package helpers

import (
	"time"
)

// TestNow returns a fixed time (2026-02-11 12:00:00 UTC) for deterministic tests (JWT expiry, logs, etc.).
//
// Parameters: none.
//
// Returns: time.Time in UTC.
//
// Called from tests (e.g. auth/token_test, service/validator_test) when a fixed "current" time is needed.
func TestNow() time.Time {
	return time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
}
