package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimeProvider_Panics(t *testing.T) {
	assert.PanicsWithValue(t, "service.time_provider.go: now is required", func() {
		NewTimeProvider(nil)
	})
}

func TestTimeProvider_Now(t *testing.T) {
	fixedTime := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	tp := NewTimeProvider(func() time.Time { return fixedTime })
	require.NotNil(t, tp)
	assert.Equal(t, fixedTime, tp.Now())
	// Multiple calls return the same value from the injected func.
	assert.Equal(t, fixedTime, tp.Now())
}

func TestTimeProvider_Now_CalledEachTime(t *testing.T) {
	callCount := 0
	tp := NewTimeProvider(func() time.Time {
		callCount++
		return time.Date(2026, 2, 21, 12, 0, 0, callCount, time.UTC)
	})
	_ = tp.Now()
	_ = tp.Now()
	assert.Equal(t, 2, callCount)
}
