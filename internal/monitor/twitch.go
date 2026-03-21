package monitor

import (
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// TwitchMonitor holds the state and logic for monitoring Twitch.
// It implements the MonitorController interface.
type TwitchMonitor struct {
	base      *BaseMonitor
	cfg       *config.TwitchConfig
	globalCfg *config.GlobalConfig
}

// NewTwitchMonitor creates a new Twitch monitor instance.
func NewTwitchMonitor(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) *TwitchMonitor {
	m := &TwitchMonitor{
		cfg:       cfg,
		globalCfg: globalCfg,
	}
	m.base = NewBaseMonitor(m)
	return m
}

// Run starts the monitoring loops by delegating to the BaseMonitor.
func (m *TwitchMonitor) Run() {
	m.base.Run()
}

// --- Implementation of MonitorController interface ---

func (m *TwitchMonitor) GetGlobalConfig() *config.GlobalConfig {
	return m.globalCfg
}

func (m *TwitchMonitor) GetStreamMonConfig() *config.StreamMonConfig {
	return &m.cfg.StreamMon
}

func (m *TwitchMonitor) GetChannels() []config.Channel {
	return m.cfg.Channels
}

func (m *TwitchMonitor) GetPollInterval() (time.Duration, error) {
	return time.ParseDuration(m.cfg.Scraper.PollInterval)
}

func (m *TwitchMonitor) GetMaxRequestsPerSecond() float64 {
	if m.cfg.Scraper.MaxRequestsPerSecond <= 0 {
		return 2 // Default: 2 requests per second
	}
	return m.cfg.Scraper.MaxRequestsPerSecond
}

func (m *TwitchMonitor) GetLogColor() string {
	return util.ColorPurple
}

func (m *TwitchMonitor) GetLogPrefix() string {
	return "Twitch"
}

func (m *TwitchMonitor) CheckChannelStatus(ch config.Channel, httpClient *http.Client) (LiveInfo, error) {
	return CheckLiveGQL(httpClient, ch.ID, m.globalCfg)
}

func (m *TwitchMonitor) BuildDownloaderCmd(ch config.Channel, status LiveInfo) *exec.Cmd {
	url := "https://www.twitch.tv/" + ch.ID
	args := append(m.cfg.StreamMon.Args, url)
	npxArgs := append([]string{"-y", "twitch-dlp"}, args...)
	cmd := exec.Command("npx", npxArgs...)
	return cmd
}

// MonitorTwitch is the public entry point that sets up and runs the monitor.
func MonitorTwitch(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) {
	monitor := NewTwitchMonitor(cfg, globalCfg)
	monitor.Run()
}
