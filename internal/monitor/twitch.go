package monitor

import (
	"fmt"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// TwitchMonitor holds the state and logic for monitoring Twitch.
type TwitchMonitor struct {
	cfg             *config.TwitchConfig
	globalCfg       *config.GlobalConfig
	liveStatus      map[string]LiveInfo  // map[channelID]LiveInfo
	activeDownloads map[string]*exec.Cmd // map[channelID]process
}

// NewTwitchMonitor creates a new Twitch monitor instance.
func NewTwitchMonitor(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) *TwitchMonitor {
	return &TwitchMonitor{
		cfg:             cfg,
		globalCfg:       globalCfg,
		liveStatus:      make(map[string]LiveInfo),
		activeDownloads: make(map[string]*exec.Cmd),
	}
}

// Run starts the monitoring loops.
func (m *TwitchMonitor) Run() {
	fmt.Printf("[%sTwitch%s] Monitor started for %d channels.\n", util.ColorPurple, util.ColorReset, len(m.cfg.Channels))
	fmt.Printf("[%sTwitch%s] Working Directory: %s\n", util.ColorPurple, util.ColorReset, m.cfg.StreamMon.WorkingDirectory)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for t := range ticker.C {
		m.checkAllChannels(t)
	}
}

// checkAllChannels simulates the main check cycle.
func (m *TwitchMonitor) checkAllChannels(t time.Time) {
	fmt.Printf("%s [%sTwitch%s] Checking live status...\n", util.FormatTime(t, m.globalCfg.Timezone), util.ColorPurple, util.ColorReset)

	// Placeholder for actual check logic
	for _, ch := range m.cfg.Channels {
		// Future logic: go m.checkLive(ch)
		_ = ch
	}
}

// MonitorTwitch is the public entry point that sets up and runs the monitor.
func MonitorTwitch(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) {
	monitor := NewTwitchMonitor(cfg, globalCfg)
	monitor.Run()
}
