package monitor

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

// YTMonitor holds the state and logic for monitoring YouTube.
// It implements the MonitorController interface.
type YTMonitor struct {
	base           *BaseMonitor
	cfg            *config.YTConfig
	globalCfg      *config.GlobalConfig
	fallbackMu     sync.RWMutex
	fallbackStates map[string]fallbackState

	stats logging.FallbackStats
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

func (m *YTMonitor) getFallbackState(channelID string) (fallbackState, bool) {
	m.fallbackMu.RLock()
	defer m.fallbackMu.RUnlock()

	state, ok := m.fallbackStates[channelID]
	return state, ok
}

func (m *YTMonitor) setFallbackState(channelID string, state fallbackState) {
	m.fallbackMu.Lock()
	defer m.fallbackMu.Unlock()

	m.fallbackStates[channelID] = state
}

func (m *YTMonitor) clearFallbackState(channelID string) {
	m.fallbackMu.Lock()
	defer m.fallbackMu.Unlock()

	delete(m.fallbackStates, channelID)
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
	return ansi.ColorRed
}

func (m *YTMonitor) GetLogPrefix() string {
	return logPrefixYouTube
}

func (m *YTMonitor) cookiesFile() string {
	return strings.TrimSpace(m.cfg.Scraper.CookiesFile)
}

func (m *YTMonitor) cookiesFileAbs() string {
	cookiesFile := m.cookiesFile()
	if cookiesFile == "" {
		return ""
	}
	absPath, err := filepath.Abs(cookiesFile)
	if err != nil {
		return cookiesFile
	}
	return absPath
}

func (m *YTMonitor) shouldCheckMembers(ch config.Channel) bool {
	return m.cfg.Scraper.MemberCheckAll || ch.MemberCheck
}

func (m *YTMonitor) GetDownloadWaitRetries() int {
	return m.cfg.Scraper.DownloadWaitRetries
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
