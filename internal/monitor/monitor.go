package monitor

import "time"

// LiveInfo stores details about a live stream.
type LiveInfo struct {
	IsLive          bool
	VideoID         string
	Title           string
	CreatedAt       time.Time
	LastBroadcastID string
}
