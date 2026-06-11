package monitor

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
)

type pendingSuccessRetryController struct {
	retryCalls int
	logPrefix  string
}

func (c *pendingSuccessRetryController) GetGlobalConfig() *config.GlobalConfig {
	return &config.GlobalConfig{}
}

func (c *pendingSuccessRetryController) GetStreamMonConfig() *config.StreamMonConfig {
	return &config.StreamMonConfig{WorkingDirectory: "download_yt"}
}

func (c *pendingSuccessRetryController) GetChannels() []config.Channel {
	return nil
}

func (c *pendingSuccessRetryController) GetPollInterval() (time.Duration, error) {
	return time.Minute, nil
}

func (c *pendingSuccessRetryController) GetMaxRequestsPerSecond() float64 {
	return 1
}

func (c *pendingSuccessRetryController) GetLogColor() string {
	return ""
}

func (c *pendingSuccessRetryController) GetLogPrefix() string {
	if c.logPrefix != "" {
		return c.logPrefix
	}
	return logPrefixYouTube
}

func (c *pendingSuccessRetryController) CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	return models.LiveInfo{}, nil
}

func (c *pendingSuccessRetryController) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	return exec.Command("yt-dlp", status.VideoID)
}

func (c *pendingSuccessRetryController) BuildRetryDownloaderCmd(ch config.Channel, status models.LiveInfo, retry ytRetryDownloader) (*exec.Cmd, string, bool) {
	c.retryCalls++
	return exec.Command("yt-dlp", status.VideoID), "yt-dlp", true
}

func TestOfflineVODRetryRequiresPriorStillLiveConfirmation(t *testing.T) {
	controller := &pendingSuccessRetryController{}
	base := NewBaseMonitor(controller)
	base.pendingYTSuccesses["channel"] = pendingYTSuccess{
		videoID:                           "video",
		completedPoll:                     1,
		completedDownloader:               "yt-dlp",
		confirmedStillLiveAfterCompletion: false,
	}

	base.resolvePendingYTSuccess(
		config.Channel{ID: "channel", Name: "Channel"},
		models.LiveInfo{IsLive: false},
		2,
	)

	if controller.retryCalls != 0 {
		t.Fatalf("offline VOD retry was requested without prior still-live confirmation")
	}
}

func TestTwitchPendingSuccessArchivesAfterNextPoll(t *testing.T) {
	controller := &pendingSuccessRetryController{logPrefix: logPrefixTwitch}
	base := NewBaseMonitor(controller)
	base.pollGeneration.Store(1)
	base.setPendingTwitchSuccess("channel", "stream-id", "twitch-dlp")

	base.resolvePendingTwitchSuccess(config.Channel{ID: "channel", Name: "Channel"}, 1)
	if base.downloadedVideos["channel"]["stream-id"] {
		t.Fatalf("twitch success archived before the next poll")
	}

	base.resolvePendingTwitchSuccess(config.Channel{ID: "channel", Name: "Channel"}, 2)
	if !base.downloadedVideos["channel"]["stream-id"] {
		t.Fatalf("twitch success was not archived on the next poll")
	}
}
