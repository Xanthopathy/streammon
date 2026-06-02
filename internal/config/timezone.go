package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func validTimezone(timezone string) bool {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return false
	}
	if _, err := time.LoadLocation(timezone); err == nil {
		return true
	}
	if after, ok := strings.CutPrefix(timezone, "UTC"); ok {
		timezone = after
	}
	_, err := parseUTCOffset(timezone)
	return err == nil
}

func parseUTCOffset(offset string) (int, error) {
	offset = strings.TrimSpace(offset)
	if offset == "" {
		return 0, nil
	}

	sign := int64(1)
	if strings.HasPrefix(offset, "-") {
		sign = -1
		offset = offset[1:]
	} else if strings.HasPrefix(offset, "+") {
		offset = offset[1:]
	}

	parts := strings.Split(offset, ":")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid offset format")
	}

	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	minutes := int64(0)
	if len(parts) == 2 {
		minutes, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, err
		}
	}

	if hours > 23 || minutes > 59 {
		return 0, fmt.Errorf("offset out of range")
	}

	return int(sign * (hours*3600 + minutes*60)), nil
}
