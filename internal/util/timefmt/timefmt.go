package timefmt

import (
	"fmt"
	"time"

	"streammon/internal/util/ansi"
	"streammon/internal/util/utcoffset"
)

// FormatTime formats a time.Time object into the standard log timestamp string.
// It respects the timezone string provided, defaulting to UTC if invalid/empty.
// Supports IANA timezone names (e.g., "Asia/Tokyo") or UTC offset format (e.g., "UTC+7", "UTC-5", "+7", "-5")
func FormatTime(t time.Time, timezone string) string {
	var loc *time.Location

	if timezone == "" {
		loc = time.UTC
	} else {
		// Try to load as IANA timezone first
		var err error
		loc, err = time.LoadLocation(timezone)

		if err != nil {
			// If that fails, try to parse as UTC offset format
			offsetSeconds, parseErr := utcoffset.Parse(timezone)
			if parseErr == nil {
				loc = time.FixedZone("UTC", offsetSeconds)
			} else {
				// Fall back to UTC if offset parsing fails
				loc = time.UTC
			}
		}
	}

	// The format "MST-07:00" includes the timezone name and offset, e.g., "UTC+00:00".
	formattedTime := t.In(loc).Format("2006-01-02 15:04:05 MST-07:00")
	return fmt.Sprintf("[%s%s%s]", ansi.ColorYellow, formattedTime, ansi.ColorReset)
}
