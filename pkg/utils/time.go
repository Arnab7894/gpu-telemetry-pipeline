package utils

import (
	"time"
)

// FormatTimestamp formats a timestamp to RFC3339
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// ParseTimestamp parses a timestamp from RFC3339 format
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// NowUTC returns the current time in UTC
func NowUTC() time.Time {
	return time.Now().UTC()
}

// StartOfDay returns the start of the day for the given time
func StartOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

// EndOfDay returns the end of the day for the given time
func EndOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 999999999, t.Location())
}

// DurationSince returns the duration since the given time
func DurationSince(t time.Time) time.Duration {
	return time.Since(t)
}

// DurationUntil returns the duration until the given time
func DurationUntil(t time.Time) time.Duration {
	return time.Until(t)
}

// AddDuration adds a duration to the given time
func AddDuration(t time.Time, d time.Duration) time.Time {
	return t.Add(d)
}

// SubtractDuration subtracts a duration from the given time
func SubtractDuration(t time.Time, d time.Duration) time.Time {
	return t.Add(-d)
}
