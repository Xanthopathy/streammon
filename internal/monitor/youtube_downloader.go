package monitor

import (
	"os/exec"

	"streammon/internal/config"
	"streammon/internal/models"
)

// BuildDownloaderCmd constructs the command to run yt-dlp.
func (m *YTMonitor) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID

	args := append([]string{}, m.cfg.StreamMon.Args...)

	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd
}

// BuildFallbackDownloaderCmd constructs the optional livestream_dl fallback.
func (m *YTMonitor) BuildFallbackDownloaderCmd(ch config.Channel, status models.LiveInfo) (*exec.Cmd, string, bool) {
	if !m.cfg.LivestreamDL.Enabled {
		return nil, "", false
	}

	args := append([]string{}, m.cfg.LivestreamDL.Args...)
	args = append(args, status.VideoID)
	return exec.Command("livestream_dl", args...), "livestream_dl", true
}
