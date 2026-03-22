package monitor

import (
	"net/http"
	"os/exec"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
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
	CheckChannelStatus(ch config.Channel, httpClient *http.Client) (models.LiveInfo, error)
	BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd
}
