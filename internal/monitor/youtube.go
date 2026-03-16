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
	return time.ParseDuration(m.cfg.Scraper.RSS.PollInterval)
}

func (m *YTMonitor) GetLogColor() string {
	return util.ColorRed
}

func (m *YTMonitor) GetLogPrefix() string {
	return "YT"
}

// CheckChannelStatus for YouTube will involve fetching and parsing the RSS feed.
// This is a placeholder for that future implementation.
func (m *YTMonitor) CheckChannelStatus(ch config.Channel, httpClient *http.Client) (LiveInfo, error) {
	// TODO: Implement YouTube RSS feed parsing logic here.
	// 1. Construct RSS feed URL: https://www.youtube.com/feeds/videos.xml?channel_id=...
	// 2. Fetch the feed using httpClient.
	// 3. Parse the XML feed.
	// 4. Find the latest <entry>.
	// 5. Check if it's a new video (compare against lastSeenVideoID).
	// 6. Check if it's a "live" or "upcoming" stream via yt:liveBroadcastContent.
	// 7. Check if it's older than `ignore_older_than` from config.
	// 8. Return a populated LiveInfo struct.
	util.DebugLog(m.globalCfg, "YouTube", fmt.Sprintf("Checking channel %s (Not Implemented)", ch.Name))
	return LiveInfo{IsLive: false}, nil // Return not live for now
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
