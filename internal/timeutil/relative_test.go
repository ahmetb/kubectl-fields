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

func TestFormatRelativeTime_DaysAndHours(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-5*24*time.Hour - 12*time.Hour)
	assert.Equal(t, "5d12h ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Weeks(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-7 * 24 * time.Hour)
	assert.Equal(t, "1w ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_WeeksAndDays(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-17 * 24 * time.Hour) // 2w + 3d
	assert.Equal(t, "2w3d ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Months(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	then := now.Add(-90 * 24 * time.Hour) // 90d = 3mo exactly
	assert.Equal(t, "3mo ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_MonthsAndWeeks(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// 104 days = 3mo (90d) + 14d = 3mo + 2w
	then := now.Add(-104 * 24 * time.Hour)
	assert.Equal(t, "3mo2w ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_Years(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// 400 days = 1y (365d) + 35d = 1y + 1mo(30d) + 5d -> "1y1mo ago"
	then := now.Add(-400 * 24 * time.Hour)
	assert.Equal(t, "1y1mo ago", FormatRelativeTime(now, then))
}

func TestFormatRelativeTime_YearsAndMonths(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// 425 days = 1y(365) + 60d = 1y + 2mo(60/30) -> "1y2mo ago"
	then := now.Add(-425 * 24 * time.Hour)
	assert.Equal(t, "1y2mo ago", FormatRelativeTime(now, then))
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
