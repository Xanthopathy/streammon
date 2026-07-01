package monitor

import (
	"context"
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/scrapers/twitch"
	"streammon/internal/util/ansi"
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
	return ansi.ColorPurple
}

func (m *TwitchMonitor) GetLogPrefix() string {
	return logPrefixTwitch
}

func (m *TwitchMonitor) CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	return twitch.CheckLiveGQL(ctx, httpClient, ch.ID, m.base.logger)
}

func (m *TwitchMonitor) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	url := "https://www.twitch.tv/" + ch.ID
	args := append([]string{}, m.cfg.TwitchDLP.Args...)
	args = append(args, url)
	npxArgs := append([]string{"-y", "twitch-dlp"}, args...)
	cmd := exec.Command("npx", npxArgs...)
	return cmd
}

// GetDownloadWaitRetries returns how many [live-from-start] "Cannot find the
// playlist" lines to tolerate before triggering a live-edge fallback.
func (m *TwitchMonitor) GetDownloadWaitRetries() int {
	return m.cfg.Scraper.DownloadWaitRetries
}

// BuildFallbackDownloaderCmd builds a twitch-dlp command with --live-from-start
// removed so the download continues from the live edge when the VOD playlist
// is unavailable. Returns (nil, "", false) if --live-from-start was not present
// in the configured args, meaning no fallback is needed.
func (m *TwitchMonitor) BuildFallbackDownloaderCmd(ch config.Channel, status models.LiveInfo) (*exec.Cmd, string, bool) {
	// Only fall back if --live-from-start was actually requested.
	hasLiveFromStart := false
	for _, a := range m.cfg.TwitchDLP.Args {
		if a == "--live-from-start" {
			hasLiveFromStart = true
			break
		}
	}
	if !hasLiveFromStart {
		return nil, "", false
	}

	// Strip --live-from-start from the args.
	filtered := make([]string, 0, len(m.cfg.TwitchDLP.Args))
	for _, a := range m.cfg.TwitchDLP.Args {
		if a != "--live-from-start" {
			filtered = append(filtered, a)
		}
	}

	url := "https://www.twitch.tv/" + ch.ID
	filtered = append(filtered, url)
	npxArgs := append([]string{"-y", "twitch-dlp"}, filtered...)
	cmd := exec.Command("npx", npxArgs...)
	return cmd, "twitch-dlp", true
}

// MonitorTwitch is the public entry point that sets up and runs the monitor.
func MonitorTwitch(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) {
	monitor := NewTwitchMonitor(cfg, globalCfg)
	monitor.Run()
}
