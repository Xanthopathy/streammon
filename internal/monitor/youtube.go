package monitor

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// YTMonitor holds the state and logic for monitoring YouTube.
// It implements the MonitorController interface.
type YTMonitor struct {
	base      *BaseMonitor
	cfg       *config.YTConfig
	globalCfg *config.GlobalConfig
	// lastSeenVideoID map[string]string // This state can be managed inside CheckChannelStatus if needed
}

// NewYTMonitor creates a new YouTube monitor instance.
func NewYTMonitor(cfg *config.YTConfig, globalCfg *config.GlobalConfig) *YTMonitor {
	m := &YTMonitor{
		cfg:       cfg,
		globalCfg: globalCfg,
		// lastSeenVideoID: make(map[string]string),
	}
	m.base = NewBaseMonitor(m)
	return m
}

// Run starts the monitoring loops by delegating to the BaseMonitor.
func (m *YTMonitor) Run() {
	// The base monitor's Run method will print the start message.
	// We can add YT specific startup logic here if needed in the future.
	m.base.Run()
}

// --- Implementation of MonitorController interface ---

func (m *YTMonitor) GetGlobalConfig() *config.GlobalConfig {
	return m.globalCfg
}

func (m *YTMonitor) GetStreamMonConfig() *config.StreamMonConfig {
	return &m.cfg.StreamMon
}

func (m *YTMonitor) GetChannels() []config.Channel {
	return m.cfg.Channels
}

func (m *YTMonitor) GetPollInterval() (time.Duration, error) {
	// YouTube uses RSS, which has a different config structure
	return time.ParseDuration(m.cfg.Scraper.PollInterval)
}

func (m *YTMonitor) GetLogColor() string {
	return util.ColorRed
}

func (m *YTMonitor) GetLogPrefix() string {
	return "YT"
}

// CheckChannelStatus for YouTube uses both HTTP redirect detection and RSS parsing
// to confirm a stream is live. This dual approach ensures:
// 1. HTTP redirect check provides real-time confirmation (302 to watch page = live)
// 2. RSS check provides metadata about the stream
// Only launches yt-dlp if both checks confirm the stream exists.
func (m *YTMonitor) CheckChannelStatus(ch config.Channel, httpClient *http.Client) (LiveInfo, error) {
	// Parse the ignore_older_than duration from config
	ignoreOlderThan, err := time.ParseDuration(m.cfg.Scraper.IgnoreOlderThan)
	if err != nil {
		// Default to 24 hours if parse fails
		ignoreOlderThan = 24 * time.Hour
		util.DebugLog(m.globalCfg, "YouTube", fmt.Sprintf("Failed to parse ignore_older_than for %s: %v, using default 24h", ch.Name, err))
	}

	return CheckLiveYouTube(httpClient, ch.ID, ch.Name, m.globalCfg, ignoreOlderThan)
}

// BuildDownloaderCmd constructs the command to run yt-dlp.
func (m *YTMonitor) BuildDownloaderCmd(ch config.Channel, status LiveInfo) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID
	// Note: yt-dlp args might need special handling for things like --paths
	args := append(m.cfg.StreamMon.Args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd
}

// MonitorYouTube is the public entry point that sets up and runs the monitor.
func MonitorYouTube(cfg *config.YTConfig, globalCfg *config.GlobalConfig) {
	monitor := NewYTMonitor(cfg, globalCfg)
	monitor.Run()
}
