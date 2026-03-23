package monitor

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/scrapers/youtube"
	"streammon/internal/util"
)

// YTMonitor holds the state and logic for monitoring YouTube.
// It implements the MonitorController interface.
type YTMonitor struct {
	base           *BaseMonitor
	cfg            *config.YTConfig
	globalCfg      *config.GlobalConfig
	fallbackStates map[string]fallbackState

	stats util.FallbackStats
}

type fallbackState struct {
	method    string
	expiresAt time.Time
}

// NewYTMonitor creates a new YouTube monitor instance.
func NewYTMonitor(cfg *config.YTConfig, globalCfg *config.GlobalConfig) *YTMonitor {
	m := &YTMonitor{
		cfg:            cfg,
		globalCfg:      globalCfg,
		fallbackStates: make(map[string]fallbackState),
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

func (m *YTMonitor) GetMaxRequestsPerSecond() float64 {
	if m.cfg.Scraper.MaxRequestsPerSecond <= 0 {
		return 2 // Default: 2 requests per second
	}
	return m.cfg.Scraper.MaxRequestsPerSecond
}

func (m *YTMonitor) GetLogColor() string {
	return util.ColorRed
}

func (m *YTMonitor) GetLogPrefix() string {
	return "YT"
}

// CheckChannelStatus for YouTube uses RSS parsing to confirm a stream is live.
// Only launches yt-dlp if both checks confirm the stream exists.
func (m *YTMonitor) CheckChannelStatus(ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	// Parse the ignore_older_than duration from config
	ignoreOlderThan, err := time.ParseDuration(m.cfg.Scraper.IgnoreOlderThan)
	if err != nil {
		// Default to 24 hours if parse fails
		ignoreOlderThan = 24 * time.Hour
		m.base.logger.Debug("YouTube", fmt.Sprintf("Failed to parse ignore_older_than for %s%s%s: %v, using default 24h", util.ColorOrange, ch.Name, util.ColorReset, err))
	}

	fallbackDuration, err := time.ParseDuration(m.cfg.Scraper.FallbackDuration)
	if err != nil {
		fallbackDuration = 15 * time.Minute
		m.base.logger.Debug("YouTube", fmt.Sprintf("Failed to parse fallback_duration, using default 15m: %v", err))
	}

	// Determine check order based on config
	// Might add Invidious/Holodex later
	var methods []string
	defaultMethod := m.cfg.Scraper.CheckMethod

	// Check if channel is in fallback mode
	currentState, hasState := m.fallbackStates[ch.ID]
	now := time.Now()
	if hasState && now.Before(currentState.expiresAt) {
		// Use the fallback method as primary
		// We assume only 2 methods for now. If persistent method is live, try live then rss.
		if currentState.method == "live" {
			methods = []string{"live", "rss"}
		} else {
			methods = []string{"rss", "live"}
		}
	} else if defaultMethod == "live" {
		methods = []string{"live", "rss"}
	} else {
		// Default to RSS first
		methods = []string{"rss", "live"}
	}

	var lastErr error

	for i, method := range methods {
		var info models.LiveInfo
		var err error

		switch method {
		case "rss":
			info, err = youtube.CheckYouTubeViaRSS(httpClient, ch.ID, ch.Name, m.base.logger, ignoreOlderThan)
		case "live":
			info, err = youtube.CheckYouTubeViaLivePage(httpClient, ch.ID, ch.Name, m.base.logger)
		default:
			continue
		}

		if err == nil {
			// If we succeeded using a method that isn't the configured default, set/refresh fallback state
			// This makes the fallback "sticky" for a duration (fallbackDuration)
			if method != defaultMethod {
				m.fallbackStates[ch.ID] = fallbackState{
					method:    method,
					expiresAt: now.Add(fallbackDuration),
				}
			} else {
				// We succeeded with the default method. Clear any fallback state to revert to normal behavior.
				delete(m.fallbackStates, ch.ID)
			}

			return info, nil
		}

		// Identify fallback method if one exists
		var fallbackName string
		if i+1 < len(methods) {
			fallbackName = methods[i+1]
			m.stats.Add(ch.Name, fallbackName)
		}

		// Log failure only if API verbose is on (to reduce spam)
		// Use "YouTubeAPI" debug type which triggers on youtube_api_verbose_debug
		m.base.logger.Debug("YouTubeAPI", fmt.Sprintf("Method '%s' failed for %s%s%s: %v. Trying fallback to '%s'...", method, util.ColorOrange, ch.Name, util.ColorReset, err, fallbackName))
		lastErr = err
	}

	return models.LiveInfo{}, fmt.Errorf("all check methods failed (last error: %w)", lastErr)
}

// BuildDownloaderCmd constructs the command to run yt-dlp.
func (m *YTMonitor) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID
	// Note: yt-dlp args might need special handling for things like --paths
	args := append(m.cfg.StreamMon.Args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd
}

// LogStats prints a summary of any failures/swaps that occurred during the check loop.
// It should be called by the main loop after checkAllChannels completes.
func (m *YTMonitor) LogStats() {
	m.stats.LogAndReset(m.base.logger)
}

// MonitorYouTube is the public entry point that sets up and runs the monitor.
func MonitorYouTube(cfg *config.YTConfig, globalCfg *config.GlobalConfig) {
	monitor := NewYTMonitor(cfg, globalCfg)
	monitor.Run()
}
