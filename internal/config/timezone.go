package config

import (
	"strings"
	"time"

	"streammon/internal/util/utcoffset"
)

func validTimezone(timezone string) bool {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return false
	}
	if _, err := time.LoadLocation(timezone); err == nil {
		return true
	}
	_, err := utcoffset.Parse(timezone)
	return err == nil
}
