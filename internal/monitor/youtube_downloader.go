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

// BuildDownloaderCmd constructs the command for the configured YouTube downloader.
func (m *YTMonitor) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	isMemberStream := status.Source == models.LiveSourceMembers
	if isMemberStream {
		return m.buildDownloaderByName(m.cfg.Scraper.MemberDownloader, status, true)
	}

	return m.buildDownloaderByName(m.cfg.Scraper.DownloaderMethod, status, false)
}

func (m *YTMonitor) buildDownloaderByName(name string, status models.LiveInfo, includeCookies bool) *exec.Cmd {
	if name == "livestream_dl" {
		return m.buildLivestreamDLCmd(status, includeCookies)
	}
	return m.buildYTDLPCmd(status, includeCookies)
}

func (m *YTMonitor) buildYTDLPCmd(status models.LiveInfo, includeCookies bool) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID

	args := append([]string{}, m.cfg.YTDLP.Args...)
	cookiesFile := m.cookiesFileAbs()
	if includeCookies && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}

	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd
}

func (m *YTMonitor) buildLivestreamDLCmd(status models.LiveInfo, includeCookies bool) *exec.Cmd {
	args := append([]string{}, m.cfg.LivestreamDL.Args...)
	cookiesFile := m.cookiesFileAbs()
	if includeCookies && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}
	args = append(args, status.VideoID)
	return exec.Command("livestream_dl", args...)
}

// BuildFallbackDownloaderCmd constructs a one-shot fallback using the other
// configured YouTube downloader when fallback rules allow it.
func (m *YTMonitor) BuildFallbackDownloaderCmd(ch config.Channel, status models.LiveInfo) (*exec.Cmd, string, bool) {
	isMemberStream := status.Source == models.LiveSourceMembers
	primary := m.cfg.Scraper.DownloaderMethod
	if isMemberStream {
		primary = m.cfg.Scraper.MemberDownloader
	}

	switch primary {
	case "livestream_dl":
		return m.buildYTDLPCmd(status, isMemberStream), "yt-dlp", true
	case "yt-dlp":
		if !isMemberStream && !m.cfg.LivestreamDL.Enabled {
			return nil, "", false
		}
		return m.buildLivestreamDLCmd(status, isMemberStream), "livestream_dl", true
	default:
		return nil, "", false
	}
}

// BuildRetryDownloaderCmd chooses a downloader different from the one that just
// completed when the same YouTube video is still live on the next poll.
func (m *YTMonitor) BuildRetryDownloaderCmd(ch config.Channel, status models.LiveInfo, avoidDownloader string) (*exec.Cmd, string, bool) {
	isMemberStream := status.Source == models.LiveSourceMembers

	switch avoidDownloader {
	case "yt-dlp":
		if !isMemberStream && !m.cfg.LivestreamDL.Enabled {
			return nil, "", false
		}
		return m.buildLivestreamDLCmd(status, isMemberStream), "livestream_dl", true
	case "livestream_dl":
		return m.buildYTDLPCmd(status, isMemberStream), "yt-dlp", true
	default:
		return nil, "", false
	}
}
