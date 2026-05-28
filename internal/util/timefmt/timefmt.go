package timefmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"streammon/internal/util/ansi"
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
			offsetStr := timezone
			// Handle "UTC+7" by removing "UTC" prefix
			if after, ok := strings.CutPrefix(offsetStr, "UTC"); ok {
				offsetStr = after
			}

			// Try to parse the offset string (e.g., "+7", "-5", "+5:30")
			offsetSeconds, parseErr := parseUTCOffset(offsetStr)
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

// parseUTCOffset parses UTC offset strings like "+7", "-5", "+5:30" and returns offset in seconds
func parseUTCOffset(offsetStr string) (int, error) {
	offsetStr = strings.TrimSpace(offsetStr)

	if offsetStr == "" {
		// Empty offset means UTC
		return 0, nil
	}

	// Determine sign
	var sign int64 = 1
	if strings.HasPrefix(offsetStr, "-") {
		sign = -1
		offsetStr = offsetStr[1:]
	} else if strings.HasPrefix(offsetStr, "+") {
		offsetStr = offsetStr[1:]
	}

	// Parse hours and optional minutes
	parts := strings.Split(offsetStr, ":")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid offset format")
	}

	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	var minutes int64 = 0
	if len(parts) == 2 {
		minutes, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, err
		}
	}

	totalSeconds := sign * (hours*3600 + minutes*60)
	return int(totalSeconds), nil
}
