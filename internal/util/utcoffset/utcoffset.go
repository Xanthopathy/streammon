package utcoffset

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse parses UTC offset strings like "", "+7", "-5", "+5:30", and "UTC+7".
// It returns the offset in seconds.
func Parse(offset string) (int, error) {
	offset = strings.TrimSpace(offset)
	if after, ok := strings.CutPrefix(offset, "UTC"); ok {
		offset = after
	}
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
