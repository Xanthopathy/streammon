package monitor

import (
	"os/exec"

	"streammon/internal/config"
	"streammon/internal/models"
)

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

	cookiesFile := m.cookiesFileAbs()
	if m.shouldUseCookiesForDownload(ch) && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}

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
	cookiesFile := m.cookiesFileAbs()
	if m.cfg.LivestreamDL.UseCookies && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}
	args = append(args, status.VideoID)
	return exec.Command("livestream_dl", args...), "livestream_dl", true
}
