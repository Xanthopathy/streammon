package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

func collectGlobalConfigWarnings(path string, meta toml.MetaData, cfg, defaults *GlobalConfig) []ConfigWarning {
	var warnings []ConfigWarning

	addMissingWarning(&warnings, path, meta, []string{"timezone"}, defaults.Timezone)
	addMissingWarning(&warnings, path, meta, []string{"max_concurrent_downloads"}, defaults.MaxConcurrentDownloads)
	addMissingWarning(&warnings, path, meta, []string{"enable_youtube"}, defaults.EnableYoutube)
	addMissingWarning(&warnings, path, meta, []string{"enable_twitch"}, defaults.EnableTwitch)
	addMissingWarning(&warnings, path, meta, []string{"youtube_verbose_debug"}, defaults.YoutubeVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"youtube_api_verbose_debug"}, defaults.YoutubeAPIVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"youtube_dlp_verbose_debug"}, defaults.YoutubeDlpVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"twitch_verbose_debug"}, defaults.TwitchVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"twitch_api_verbose_debug"}, defaults.TwitchAPIVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"twitch_dlp_verbose_debug"}, defaults.TwitchDlpVerboseDebug)
	addMissingWarning(&warnings, path, meta, []string{"youtube_archive_downloads"}, defaults.YoutubeArchiveDownloads)
	addMissingWarning(&warnings, path, meta, []string{"twitch_archive_downloads"}, defaults.TwitchArchiveDownloads)
	addMissingWarning(&warnings, path, meta, []string{"save_download_logs"}, defaults.SaveDownloadLogs)
	addMissingWarning(&warnings, path, meta, []string{"subprocess_progress_interval"}, defaults.SubprocessProgressInterval)
	addMissingWarning(&warnings, path, meta, []string{"subprocess_wait_interval"}, defaults.SubprocessWaitInterval)
	addMissingWarning(&warnings, path, meta, []string{"clear_all_lockfiles"}, defaults.ClearAllLockfiles)

	if !validTimezone(cfg.Timezone) {
		addInvalidWarning(&warnings, path, "timezone", cfg.Timezone, defaults.Timezone, "must be an IANA timezone or UTC offset like UTC+7")
		cfg.Timezone = defaults.Timezone
	}
	if cfg.MaxConcurrentDownloads <= 0 {
		addInvalidWarning(&warnings, path, "max_concurrent_downloads", cfg.MaxConcurrentDownloads, defaults.MaxConcurrentDownloads, "must be greater than 0")
		cfg.MaxConcurrentDownloads = defaults.MaxConcurrentDownloads
	}
	if cfg.SubprocessProgressInterval < 0 {
		addInvalidWarning(&warnings, path, "subprocess_progress_interval", cfg.SubprocessProgressInterval, defaults.SubprocessProgressInterval, "must be 0 or greater")
		cfg.SubprocessProgressInterval = defaults.SubprocessProgressInterval
	}
	if cfg.SubprocessWaitInterval < 0 {
		addInvalidWarning(&warnings, path, "subprocess_wait_interval", cfg.SubprocessWaitInterval, defaults.SubprocessWaitInterval, "must be 0 or greater")
		cfg.SubprocessWaitInterval = defaults.SubprocessWaitInterval
	}

	addUndecodedWarnings(&warnings, path, meta)
	return warnings
}

func collectYTConfigWarnings(path string, meta toml.MetaData, cfg, defaults *YTConfig) []ConfigWarning {
	var warnings []ConfigWarning

	addMissingWarning(&warnings, path, meta, []string{"streammon", "working_directory"}, defaults.StreamMon.WorkingDirectory)
	addMissingWarning(&warnings, path, meta, []string{"streammon", "args"}, defaults.StreamMon.Args)
	addMissingWarning(&warnings, path, meta, []string{"livestream_dl", "enabled"}, defaults.LivestreamDL.Enabled)
	addMissingWarning(&warnings, path, meta, []string{"livestream_dl", "args"}, defaults.LivestreamDL.Args)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "poll_interval"}, defaults.Scraper.PollInterval)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "ignore_older_than"}, defaults.Scraper.IgnoreOlderThan)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "max_requests_per_second"}, defaults.Scraper.MaxRequestsPerSecond)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "check_method"}, defaults.Scraper.CheckMethod)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "fallback_duration"}, defaults.Scraper.FallbackDuration)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "cookies_file"}, defaults.Scraper.CookiesFile)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "member_check_all"}, defaults.Scraper.MemberCheckAll)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "member_downloader"}, defaults.Scraper.MemberDownloader)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "download_wait_retries"}, defaults.Scraper.DownloadWaitRetries)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "member_check_args"}, defaults.Scraper.MemberCheckArgs)

	if strings.TrimSpace(cfg.StreamMon.WorkingDirectory) == "" {
		addInvalidWarning(&warnings, path, "streammon.working_directory", cfg.StreamMon.WorkingDirectory, defaults.StreamMon.WorkingDirectory, "must not be empty")
		cfg.StreamMon.WorkingDirectory = defaults.StreamMon.WorkingDirectory
	}
	if len(cfg.StreamMon.Args) == 0 {
		addInvalidWarning(&warnings, path, "streammon.args", cfg.StreamMon.Args, defaults.StreamMon.Args, "must include downloader arguments")
		cfg.StreamMon.Args = defaults.StreamMon.Args
	}
	if cfg.LivestreamDL.Enabled && len(cfg.LivestreamDL.Args) == 0 {
		addInvalidWarning(&warnings, path, "livestream_dl.args", cfg.LivestreamDL.Args, defaults.LivestreamDL.Args, "must include downloader arguments when livestream_dl fallback is enabled")
		cfg.LivestreamDL.Args = defaults.LivestreamDL.Args
	}
	validateDuration(&warnings, path, "scraper.poll_interval", &cfg.Scraper.PollInterval, defaults.Scraper.PollInterval)
	validateDuration(&warnings, path, "scraper.ignore_older_than", &cfg.Scraper.IgnoreOlderThan, defaults.Scraper.IgnoreOlderThan)
	validateDuration(&warnings, path, "scraper.fallback_duration", &cfg.Scraper.FallbackDuration, defaults.Scraper.FallbackDuration)
	if cfg.Scraper.MaxRequestsPerSecond <= 0 {
		addInvalidWarning(&warnings, path, "scraper.max_requests_per_second", cfg.Scraper.MaxRequestsPerSecond, defaults.Scraper.MaxRequestsPerSecond, "must be greater than 0")
		cfg.Scraper.MaxRequestsPerSecond = defaults.Scraper.MaxRequestsPerSecond
	}
	if cfg.Scraper.CheckMethod != "rss" && cfg.Scraper.CheckMethod != "live" {
		addInvalidWarning(&warnings, path, "scraper.check_method", cfg.Scraper.CheckMethod, defaults.Scraper.CheckMethod, `must be "rss" or "live"`)
		cfg.Scraper.CheckMethod = defaults.Scraper.CheckMethod
	}
	cfg.Scraper.MemberDownloader = strings.TrimSpace(cfg.Scraper.MemberDownloader)
	if cfg.Scraper.MemberDownloader != "livestream_dl" && cfg.Scraper.MemberDownloader != "yt-dlp" {
		addInvalidWarning(&warnings, path, "scraper.member_downloader", cfg.Scraper.MemberDownloader, defaults.Scraper.MemberDownloader, `must be "livestream_dl" or "yt-dlp"`)
		cfg.Scraper.MemberDownloader = defaults.Scraper.MemberDownloader
	}
	if cfg.Scraper.DownloadWaitRetries < 0 {
		addInvalidWarning(&warnings, path, "scraper.download_wait_retries", cfg.Scraper.DownloadWaitRetries, defaults.Scraper.DownloadWaitRetries, "must be 0 or greater")
		cfg.Scraper.DownloadWaitRetries = defaults.Scraper.DownloadWaitRetries
	}
	if usesYouTubeMemberChecks(cfg) && strings.TrimSpace(cfg.Scraper.CookiesFile) == "" {
		addInvalidWarning(&warnings, path, "scraper.cookies_file", cfg.Scraper.CookiesFile, defaults.Scraper.CookiesFile, "must not be empty when member checks are enabled")
		cfg.Scraper.CookiesFile = defaults.Scraper.CookiesFile
	}
	addChannelWarnings(&warnings, path, cfg.Channels)

	addUndecodedWarnings(&warnings, path, meta)
	return warnings
}

func usesYouTubeMemberChecks(cfg *YTConfig) bool {
	if cfg.Scraper.MemberCheckAll {
		return true
	}
	for _, ch := range cfg.Channels {
		if ch.MemberCheck {
			return true
		}
	}
	return false
}

func collectTwitchConfigWarnings(path string, meta toml.MetaData, cfg, defaults *TwitchConfig) []ConfigWarning {
	var warnings []ConfigWarning

	addMissingWarning(&warnings, path, meta, []string{"streammon", "working_directory"}, defaults.StreamMon.WorkingDirectory)
	addMissingWarning(&warnings, path, meta, []string{"streammon", "args"}, defaults.StreamMon.Args)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "poll_interval"}, defaults.Scraper.PollInterval)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "max_requests_per_second"}, defaults.Scraper.MaxRequestsPerSecond)

	if strings.TrimSpace(cfg.StreamMon.WorkingDirectory) == "" {
		addInvalidWarning(&warnings, path, "streammon.working_directory", cfg.StreamMon.WorkingDirectory, defaults.StreamMon.WorkingDirectory, "must not be empty")
		cfg.StreamMon.WorkingDirectory = defaults.StreamMon.WorkingDirectory
	}
	if len(cfg.StreamMon.Args) == 0 {
		addInvalidWarning(&warnings, path, "streammon.args", cfg.StreamMon.Args, defaults.StreamMon.Args, "must include downloader arguments")
		cfg.StreamMon.Args = defaults.StreamMon.Args
	}
	validateDuration(&warnings, path, "scraper.poll_interval", &cfg.Scraper.PollInterval, defaults.Scraper.PollInterval)
	if cfg.Scraper.MaxRequestsPerSecond <= 0 {
		addInvalidWarning(&warnings, path, "scraper.max_requests_per_second", cfg.Scraper.MaxRequestsPerSecond, defaults.Scraper.MaxRequestsPerSecond, "must be greater than 0")
		cfg.Scraper.MaxRequestsPerSecond = defaults.Scraper.MaxRequestsPerSecond
	}
	addChannelWarnings(&warnings, path, cfg.Channels)

	addUndecodedWarnings(&warnings, path, meta)
	return warnings
}

func addChannelWarnings(warnings *[]ConfigWarning, path string, channels []Channel) {
	for i, ch := range channels {
		prefix := fmt.Sprintf("channel[%d]", i)
		if strings.TrimSpace(ch.ID) == "" {
			*warnings = append(*warnings, ConfigWarning{
				Path:    path,
				Key:     prefix + ".id",
				Message: fmt.Sprintf("invalid %s.id=%s (must not be empty); channel may not work", prefix, formatDefault(ch.ID)),
			})
		}
		if strings.TrimSpace(ch.Name) == "" {
			*warnings = append(*warnings, ConfigWarning{
				Path:    path,
				Key:     prefix + ".name",
				Message: fmt.Sprintf("invalid %s.name=%s (must not be empty); logs will be harder to read", prefix, formatDefault(ch.Name)),
			})
		}
	}
}

func validateDuration(warnings *[]ConfigWarning, path, key string, value *string, defaultValue string) {
	if _, err := time.ParseDuration(*value); err != nil {
		addInvalidWarning(warnings, path, key, *value, defaultValue, "must be a Go duration like 30s, 15m, or 24h")
		*value = defaultValue
	}
}
