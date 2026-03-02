package monitor

import (
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// TwitchMonitor holds the state and logic for monitoring Twitch.
type TwitchMonitor struct {
	cfg             *config.TwitchConfig
	globalCfg       *config.GlobalConfig
	httpClient      *http.Client
	statusMutex     sync.RWMutex
	liveStatus      map[string]LiveInfo  // map[channelID]LiveInfo
	activeDownloads map[string]*exec.Cmd // map[channelID]process
}

// NewTwitchMonitor creates a new Twitch monitor instance.
func NewTwitchMonitor(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) *TwitchMonitor {
	// Create a persistent HTTP client for reuse
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &TwitchMonitor{
		cfg:             cfg,
		globalCfg:       globalCfg,
		httpClient:      httpClient,
		liveStatus:      make(map[string]LiveInfo),
		activeDownloads: make(map[string]*exec.Cmd),
	}
}

// Run starts the monitoring loops.
func (m *TwitchMonitor) Run() {
	fmt.Printf("%s [%sTwitch%s] Monitor started for %d channels.\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, len(m.cfg.Channels))
	fmt.Printf("%s [%sTwitch%s] Working Directory: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, m.cfg.StreamMon.WorkingDirectory)

	ticker := time.NewTicker(60 * time.Second) // todo: make timer also a config in config_twitch.toml
	defer ticker.Stop()

	// Run initial check immediately
	m.checkAllChannels(time.Now())

	for t := range ticker.C {
		m.checkAllChannels(t) // Then check on every tick
	}
}

// checkAllChannels concurrently checks all configured Twitch channels.
func (m *TwitchMonitor) checkAllChannels(t time.Time) {
	fmt.Printf("%s [%sTwitch%s] Checking live status for %d channels...\n", util.FormatTime(t, m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, len(m.cfg.Channels))

	var wg sync.WaitGroup
	for _, ch := range m.cfg.Channels {
		wg.Add(1)
		go m.checkChannel(ch, &wg)
	}
	wg.Wait()
}

// checkChannel is the core logic for checking a single channel's status.
func (m *TwitchMonitor) checkChannel(ch config.Channel, wg *sync.WaitGroup) {
	defer wg.Done()

	newStatus, err := CheckLiveGQL(m.httpClient, ch.ID)
	if err != nil {
		fmt.Printf("%s [%sTwitch%s] Error checking %s: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
		return
	}

	m.statusMutex.Lock()
	defer m.statusMutex.Unlock()

	previousStatus, wasTracked := m.liveStatus[ch.ID]

	// Handle state changes
	if newStatus.IsLive {
		// Filter check
		matchesFilter := false
		for _, filter := range ch.Filters {
			if matched, _ := regexp.MatchString(filter, newStatus.Title); matched {
				matchesFilter = true
				break
			}
		}

		if !matchesFilter {
			// It's live, but we don't care about this title.
			return
		}

		// New stream or came back online
		if !wasTracked || !previousStatus.IsLive {
			fmt.Printf("%s [%sTwitch%s] %s%s is now LIVE%s: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, util.ColorGreen, ch.Name, util.ColorReset, newStatus.Title)
		}
		m.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline
		if wasTracked && previousStatus.IsLive {
			fmt.Printf("%s [%sTwitch%s] %s%s has gone OFFLINE%s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, util.ColorRed, ch.Name, util.ColorReset)
		}
		m.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
}

// MonitorTwitch is the public entry point that sets up and runs the monitor.
func MonitorTwitch(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) {
	monitor := NewTwitchMonitor(cfg, globalCfg)
	monitor.Run()
}
