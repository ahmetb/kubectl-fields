package timeutil

import (
	"fmt"
	"time"
)

// FormatRelativeTime returns a human-readable relative time string with
// two-unit granularity, such as "5d12h ago" or "2w3d ago".
//
// Units extracted in order: years (365d), months (30d), weeks (7d), days,
// hours, minutes, seconds. The two largest non-zero units are concatenated.
// If only one unit is non-zero, a single unit is shown (e.g., "5d ago").
//
// Edge cases: 0 seconds -> "0s ago", future timestamps -> "just now".
func FormatRelativeTime(now, then time.Time) string {
	d := now.Sub(then)

	// Future timestamps
	if d < 0 {
		return "just now"
	}

	totalSeconds := int(d.Seconds())

	// Decompose into units from largest to smallest
	years := totalSeconds / (365 * 24 * 3600)
	totalSeconds -= years * 365 * 24 * 3600

	months := totalSeconds / (30 * 24 * 3600)
	totalSeconds -= months * 30 * 24 * 3600

	weeks := totalSeconds / (7 * 24 * 3600)
	totalSeconds -= weeks * 7 * 24 * 3600

	days := totalSeconds / (24 * 3600)
	totalSeconds -= days * 24 * 3600

	hours := totalSeconds / 3600
	totalSeconds -= hours * 3600

	minutes := totalSeconds / 60
	totalSeconds -= minutes * 60

	seconds := totalSeconds

	// Build ordered list of (value, suffix) pairs
	units := []struct {
		val    int
		suffix string
	}{
		{years, "y"},
		{months, "mo"},
		{weeks, "w"},
		{days, "d"},
		{hours, "h"},
		{minutes, "m"},
		{seconds, "s"},
	}

	// Collect the first two non-zero units
	var parts []string
	for _, u := range units {
		if u.val > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", u.val, u.suffix))
			if len(parts) == 2 {
				break
			}
		}
	}

	if len(parts) == 0 {
		return "0s ago"
	}

	result := ""
	for _, p := range parts {
		result += p
	}
	return result + " ago"
}
