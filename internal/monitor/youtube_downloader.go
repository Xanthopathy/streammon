package monitor

import (
	"os/exec"
	"strings"
	"time"

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
		return m.buildDownloaderByName(m.cfg.Scraper.MemberDownloader, ch, status, true)
	}

	return m.buildDownloaderByName(m.cfg.Scraper.DownloaderMethod, ch, status, false)
}

func (m *YTMonitor) buildDownloaderByName(name string, ch config.Channel, status models.LiveInfo, includeCookies bool) *exec.Cmd {
	if name == "livestream_dl" {
		return m.buildLivestreamDLCmd(ch, status, includeCookies)
	}
	return m.buildYTDLPCmd(ch, status, includeCookies)
}

func (m *YTMonitor) buildYTDLPCmd(ch config.Channel, status models.LiveInfo, includeCookies bool) *exec.Cmd {
	args := append([]string{}, m.cfg.YTDLP.Args...)
	args = append(args, ch.AdditionalArgs...)
	return m.buildYTDLPCmdWithArgs(status, includeCookies, args)
}

func (m *YTMonitor) buildYTDLPCmdWithArgs(status models.LiveInfo, includeCookies bool, args []string) *exec.Cmd {
	url := "https://www.youtube.com/watch?v=" + status.VideoID

	cookiesFile := m.cookiesFileAbs()
	if includeCookies && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}

	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd
}

func (m *YTMonitor) buildLivestreamDLCmd(ch config.Channel, status models.LiveInfo, includeCookies bool) *exec.Cmd {
	args := append([]string{}, m.cfg.LivestreamDL.Args...)
	args = append(args, ch.AdditionalArgs...)
	cookiesFile := m.cookiesFileAbs()
	if includeCookies && cookiesFile != "" && !hasCookieArg(args) {
		args = append(args, "--cookies", cookiesFile)
	}
	args = append(args, status.VideoID)
	return exec.Command("livestream_dl", args...)
}

func buildTimestampedOutputArgs(args []string, label string) []string {
	result := append([]string{}, args...)
	suffix := " [" + label + "-" + time.Now().Format("20060102-150405") + "]"

	for i := 0; i < len(result); i++ {
		arg := result[i]
		if arg == "--output" || arg == "-o" {
			if i+1 < len(result) {
				result[i+1] = appendOutputSuffix(result[i+1], suffix)
			}
			return result
		}
		for _, prefix := range []string{"--output=", "-o="} {
			if strings.HasPrefix(arg, prefix) {
				result[i] = prefix + appendOutputSuffix(strings.TrimPrefix(arg, prefix), suffix)
				return result
			}
		}
	}

	return append(result, "--output", "[%(upload_date)s] [%(id)s] [%(title)s] [%(channel)s]"+suffix+".%(ext)s")
}

func appendOutputSuffix(template, suffix string) string {
	if strings.Contains(template, ".%(ext)s") {
		return strings.Replace(template, ".%(ext)s", suffix+".%(ext)s", 1)
	}
	return template + suffix
}

func removeLiveWaitArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--live-from-start":
			continue
		case arg == "--wait-for-video":
			if i+1 < len(args) {
				i++
			}
			continue
		case strings.HasPrefix(arg, "--wait-for-video="):
			continue
		default:
			result = append(result, arg)
		}
	}
	return result
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
		return m.buildYTDLPCmd(ch, status, isMemberStream), "yt-dlp", true
	case "yt-dlp":
		if !isMemberStream && !m.cfg.LivestreamDL.Enabled {
			return nil, "", false
		}
		return m.buildLivestreamDLCmd(ch, status, isMemberStream), "livestream_dl", true
	default:
		return nil, "", false
	}
}

// BuildRetryDownloaderCmd chooses a downloader different from the one that just
// completed when the same YouTube video is still live on the next poll.
func (m *YTMonitor) BuildRetryDownloaderCmd(ch config.Channel, status models.LiveInfo, retry ytRetryDownloader) (*exec.Cmd, string, bool) {
	isMemberStream := status.Source == models.LiveSourceMembers

	switch retry.mode {
	case ytRetryModeAlternate:
		switch retry.completedDownloader {
		case "yt-dlp":
			if !isMemberStream && !m.cfg.LivestreamDL.Enabled {
				return nil, "", false
			}
			return m.buildLivestreamDLCmd(ch, status, isMemberStream), "livestream_dl", true
		case "livestream_dl":
			return m.buildYTDLPCmd(ch, status, isMemberStream), "yt-dlp", true
		default:
			return nil, "", false
		}
	case ytRetryModeSameTimestamp:
		if !m.cfg.Scraper.RetrySameDownloaderWithTimestampWhenLive {
			return nil, "", false
		}
		switch retry.completedDownloader {
		case "yt-dlp":
			args := append([]string{}, m.cfg.YTDLP.Args...)
			args = append(args, ch.AdditionalArgs...)
			args = buildTimestampedOutputArgs(args, "live-retry")
			return m.buildYTDLPCmdWithArgs(status, isMemberStream, args), "yt-dlp", true
		case "livestream_dl":
			args := append([]string{}, m.cfg.LivestreamDL.Args...)
			args = append(args, ch.AdditionalArgs...)
			args = buildTimestampedOutputArgs(args, "live-retry")
			cookiesFile := m.cookiesFileAbs()
			if isMemberStream && cookiesFile != "" && !hasCookieArg(args) {
				args = append(args, "--cookies", cookiesFile)
			}
			args = append(args, status.VideoID)
			return exec.Command("livestream_dl", args...), "livestream_dl", true
		default:
			return nil, "", false
		}
	case ytRetryModeOfflineVOD:
		if !m.cfg.Scraper.RetryOfflineWithoutLiveArgs {
			return nil, "", false
		}
		args := append([]string{}, m.cfg.YTDLP.Args...)
		args = append(args, ch.AdditionalArgs...)
		args = removeLiveWaitArgs(args)
		args = buildTimestampedOutputArgs(args, "vod-retry")
		return m.buildYTDLPCmdWithArgs(status, isMemberStream, args), "yt-dlp", true
	default:
		return nil, "", false
	}
}
