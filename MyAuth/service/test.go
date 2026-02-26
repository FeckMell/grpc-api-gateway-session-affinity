package service

import (
	"time"
)

func TestNow() time.Time {
	return time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
}
