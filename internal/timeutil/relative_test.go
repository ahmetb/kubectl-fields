package timeutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatRelativeTime_Seconds(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-45 * time.Second)
	assert.Equal(t, "45s ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Minutes(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-5 * time.Minute)
	assert.Equal(t, "5m ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_MinutesAndSeconds(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-3*time.Minute - 10*time.Second)
	assert.Equal(t, "3m10s ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Hours(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-2 * time.Hour)
	assert.Equal(t, "2h ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_HoursAndMinutes(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-3*time.Hour - 10*time.Minute)
	assert.Equal(t, "3h10m ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Days(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-5 * 24 * time.Hour)
	assert.Equal(t, "5d ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Months(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-90 * 24 * time.Hour)
	assert.Equal(t, "3mo ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Years(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-400 * 24 * time.Hour)
	assert.Equal(t, "1y ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Future(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(10 * time.Second)
	assert.Equal(t, "just now", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Zero(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "0s ago", FormatRelativeTime(now, now))
}
