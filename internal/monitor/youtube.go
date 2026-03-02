package monitor

import (
	"fmt"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// YTMonitor holds the state and logic for monitoring YouTube.
type YTMonitor struct {
	cfg             *config.YTConfig
	liveStatus      map[string]LiveInfo  // map[channelID]LiveInfo
	lastSeenVideoID map[string]string    // map[channelID]videoID
	activeDownloads map[string]*exec.Cmd // map[channelID]process
}

// NewYTMonitor creates a new YouTube monitor instance.
func NewYTMonitor(cfg *config.YTConfig) *YTMonitor {
	return &YTMonitor{
		cfg:             cfg,
		liveStatus:      make(map[string]LiveInfo),
		lastSeenVideoID: make(map[string]string),
		activeDownloads: make(map[string]*exec.Cmd),
	}
}

// Run starts the monitoring loops.
func (m *YTMonitor) Run() {
	fmt.Printf("[%sYT%s] Monitor started for %d channels.\n", util.ColorRed, util.ColorReset, len(m.cfg.Channels))
	fmt.Printf("[%sYT%s] Working Directory: %s\n", util.ColorRed, util.ColorReset, m.cfg.StreamMon.WorkingDirectory)

	// In the future, we will spawn separate goroutines for fast-track, slow-track, and download management.
	// For now, we keep the simple simulation loop.
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for t := range ticker.C {
		m.checkAllChannels(t)
	}
}

// checkAllChannels simulates the main check cycle.
func (m *YTMonitor) checkAllChannels(t time.Time) {
	fmt.Printf("%s [%sYT%s] Checking RSS feeds...\n", util.FormatTime(t, m.cfg.StreamMon.Timezone), util.ColorRed, util.ColorReset)

	// Placeholder for actual check logic
	for _, ch := range m.cfg.Channels {
		// Future logic: go m.checkRSS(ch)
		_ = ch
	}
}

// MonitorYouTube is the public entry point that sets up and runs the monitor.
func MonitorYouTube(cfg *config.YTConfig) {
	monitor := NewYTMonitor(cfg)
	monitor.Run()
}
