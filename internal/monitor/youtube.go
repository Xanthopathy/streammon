package monitor

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/scrapers/youtube"
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
	return "YT"
}

func (m *YTMonitor) cookiesFile() string {
	return strings.TrimSpace(m.cfg.Scraper.CookiesFile)
}

func (m *YTMonitor) shouldUseCookiesForDownload(ch config.Channel) bool {
	return m.cfg.Scraper.UseCookiesForDownloads || ch.UseCookiesForDownloads
}

func (m *YTMonitor) shouldCheckMembers(ch config.Channel) bool {
	return m.cfg.Scraper.MemberCheckAll || ch.MemberCheck
}

func (m *YTMonitor) checkMembersIfEnabled(ctx context.Context, ch config.Channel) (models.LiveInfo, error) {
	if !m.shouldCheckMembers(ch) {
		return models.LiveInfo{IsLive: false}, nil
	}

	memberInfo, memberErr := youtube.CheckYouTubeViaMembersPlaylist(
		ctx,
		m.cookiesFile(),
		m.cfg.Scraper.MemberCheckArgs,
		ch.ID,
		ch.Name,
		m.base.logger,
	)
	if memberErr != nil {
		m.base.logger.Debug(
			"YouTubeAPI",
			fmt.Sprintf(
				"Member check failed for %s%s%s: %v",
				ansi.ColorOrange,
				ch.Name,
				ansi.ColorReset,
				memberErr,
			),
		)
		return models.LiveInfo{IsLive: false}, memberErr
	}

	return memberInfo, nil
}

// CheckChannelStatus for YouTube uses RSS parsing to confirm a stream is live.
// Only launches yt-dlp if both checks confirm the stream exists.
func (m *YTMonitor) CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	// Parse the ignore_older_than duration from config
	ignoreOlderThan, err := time.ParseDuration(m.cfg.Scraper.IgnoreOlderThan)
	if err != nil {
		// Default to 24 hours if parse fails
		ignoreOlderThan = 24 * time.Hour
		m.base.logger.Debug("YouTube", fmt.Sprintf("Failed to parse ignore_older_than for %s%s%s: %v, using default 24h", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
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
	currentState, hasState := m.getFallbackState(ch.ID)
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
		if ctx.Err() != nil {
			return models.LiveInfo{}, ctx.Err()
		}

		var info models.LiveInfo
		var err error

		switch method {
		case "rss":
			info, err = youtube.CheckYouTubeViaRSS(ctx, httpClient, ch.ID, ch.Name, m.base.logger, ignoreOlderThan)
		case "live":
			info, err = youtube.CheckYouTubeViaLivePage(ctx, httpClient, ch.ID, ch.Name, m.base.logger)
		default:
			continue
		}

		if err == nil {
			// If we succeeded using a method that isn't the configured default, set/refresh fallback state
			// This makes the fallback "sticky" for a duration (fallbackDuration)
			if method != defaultMethod {
				m.setFallbackState(ch.ID, fallbackState{
					method:    method,
					expiresAt: now.Add(fallbackDuration),
				})
			} else {
				// We succeeded with the default method. Clear any fallback state to revert to normal behavior.
				m.clearFallbackState(ch.ID)
			}

			if !info.IsLive {
				memberInfo, memberErr := m.checkMembersIfEnabled(ctx, ch)
				if memberErr == nil && memberInfo.IsLive {
					return memberInfo, nil
				}
			}

			return info, nil
		}

		if isConnectivityError(err) {
			return models.LiveInfo{}, err
		}

		// Identify fallback method if one exists
		var fallbackName string
		if i+1 < len(methods) {
			fallbackName = methods[i+1]
			m.stats.Add(ch.Name, fallbackName)
		}

		// Log failure only if API verbose is on (to reduce spam)
		// Use "YouTubeAPI" debug type which triggers on youtube_api_verbose_debug
		m.base.logger.Debug("YouTubeAPI", fmt.Sprintf("Method '%s' failed for %s%s%s: %v. Trying fallback to '%s'...", method, ansi.ColorOrange, ch.Name, ansi.ColorReset, err, fallbackName))
		lastErr = err
	}

	memberInfo, memberErr := m.checkMembersIfEnabled(ctx, ch)
	if memberErr == nil && memberInfo.IsLive {
		return memberInfo, nil
	}

	return models.LiveInfo{}, fmt.Errorf("all check methods failed (last error: %w)", lastErr)
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasCookieArg(args []string) bool {
	return hasArg(args, "--cookies") || hasArg(args, "--cookies-from-browser")
}

// BuildDownloaderCmd constructs the command to run yt-dlp.
func (m *YTMonitor) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID

	args := append([]string{}, m.cfg.StreamMon.Args...)

	if m.shouldUseCookiesForDownload(ch) && m.cookiesFile() != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", m.cookiesFile())
	}

	args = append(args, url)
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
