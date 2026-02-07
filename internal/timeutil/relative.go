package timeutil

import (
	"fmt"
	"time"
)

// FormatRelativeTime returns a human-readable relative time string
// describing the duration between now and then, such as "5m ago" or "3d ago".
func FormatRelativeTime(now, then time.Time) string {
	d := now.Sub(then)

	// Future timestamps
	if d < 0 {
		return "just now"
	}

	totalSeconds := int(d.Seconds())

	// Less than 60 seconds
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds ago", totalSeconds)
	}

	// Less than 1 hour
	totalMinutes := int(d.Minutes())
	if totalMinutes < 60 {
		secs := totalSeconds - totalMinutes*60
		if secs > 0 {
			return fmt.Sprintf("%dm%ds ago", totalMinutes, secs)
		}
		return fmt.Sprintf("%dm ago", totalMinutes)
	}

	// Less than 24 hours
	totalHours := int(d.Hours())
	if totalHours < 24 {
		mins := totalMinutes - totalHours*60
		if mins > 0 {
			return fmt.Sprintf("%dh%dm ago", totalHours, mins)
		}
		return fmt.Sprintf("%dh ago", totalHours)
	}

	totalDays := totalHours / 24

	// Less than 30 days
	if totalDays < 30 {
		return fmt.Sprintf("%dd ago", totalDays)
	}

	// Less than 365 days
	if totalDays < 365 {
		months := totalDays / 30
		return fmt.Sprintf("%dmo ago", months)
	}

	// 365+ days
	years := totalDays / 365
	return fmt.Sprintf("%dy ago", years)
}
