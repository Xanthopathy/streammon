package monitor

import (
	"context"
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
)

const (
	logPrefixYouTube = "YT"
	logPrefixTwitch  = "Twitch"
)

// MonitorController defines the platform-specific logic that a monitor must implement.
type MonitorController interface {
	// Getters for configuration and identity
	GetGlobalConfig() *config.GlobalConfig
	GetStreamMonConfig() *config.StreamMonConfig
	GetChannels() []config.Channel
	GetPollInterval() (time.Duration, error)
	GetMaxRequestsPerSecond() float64
	GetLogColor() string
	GetLogPrefix() string

	// Core platform-specific logic
	CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error)
	BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd
}

// FallbackDownloaderController is implemented by platforms that can retry a
// failed primary download with a different downloader.
type FallbackDownloaderController interface {
	BuildFallbackDownloaderCmd(ch config.Channel, status models.LiveInfo) (*exec.Cmd, string, bool)
}

// RetryDownloaderController is implemented by platforms that can choose an
// alternate downloader when a completed download is discovered to still be live.
type RetryDownloaderController interface {
	BuildRetryDownloaderCmd(ch config.Channel, status models.LiveInfo, retry ytRetryDownloader) (*exec.Cmd, string, bool)
}
