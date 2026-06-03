package models

import "time"

const (
	LiveSourceMembers = "members"
)

// LiveInfo stores details about a live stream.
// It is the common data structure returned by scrapers and used by the monitor.
type LiveInfo struct {
	IsLive          bool
	VideoID         string
	Title           string
	CreatedAt       time.Time
	LastBroadcastID string
	Source          string
}
