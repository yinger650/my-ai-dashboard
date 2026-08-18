// Package shared holds small cross-cutting helpers used by server and client.
package shared

import (
	"time"

	"github.com/google/uuid"
)

// NewID returns a UUIDv7 string, used for all application-generated primary keys.
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only errors if the system clock/entropy fails; fall back to v4.
		return uuid.NewString()
	}
	return id.String()
}

// IsUUID reports whether s is a syntactically valid UUID.
func IsUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// RFC3339Milli is the canonical timestamp format: UTC, millisecond precision.
const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// NowUTC returns the current time in UTC.
func NowUTC() time.Time { return time.Now().UTC() }

// FormatTime formats t as a UTC RFC3339 string with millisecond precision.
func FormatTime(t time.Time) string {
	return t.UTC().Format(RFC3339Milli)
}

// ParseTime parses an RFC3339 timestamp.
func ParseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
