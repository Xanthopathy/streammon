package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// --- Shared Structures ---

type GlobalConfig struct {
	Timezone                   string `toml:"timezone"`
	MaxConcurrentDownloads     int    `toml:"max_concurrent_downloads"`
	EnableYoutube              bool   `toml:"enable_youtube"`
	EnableTwitch               bool   `toml:"enable_twitch"`
	YoutubeVerboseDebug        bool   `toml:"youtube_verbose_debug"`
	YoutubeAPIVerboseDebug     bool   `toml:"youtube_api_verbose_debug"`
	YoutubeDlpVerboseDebug     bool   `toml:"youtube_dlp_verbose_debug"`
	TwitchVerboseDebug         bool   `toml:"twitch_verbose_debug"`
	TwitchAPIVerboseDebug      bool   `toml:"twitch_api_verbose_debug"`
	TwitchDlpVerboseDebug      bool   `toml:"twitch_dlp_verbose_debug"`
	YoutubeArchiveDownloads    bool   `toml:"youtube_archive_downloads"`
	TwitchArchiveDownloads     bool   `toml:"twitch_archive_downloads"`
	SaveDownloadLogs           bool   `toml:"save_download_logs"`
	SubprocessProgressInterval int    `toml:"subprocess_progress_interval"`
	SubprocessWaitInterval     int    `toml:"subprocess_wait_interval"`
	ClearAllLockfiles          bool   `toml:"clear_all_lockfiles"`
}

type StreamMonConfig struct {
	WorkingDirectory string   `toml:"working_directory"`
	Args             []string `toml:"args"`
}

type Channel struct {
	ID                     string   `toml:"id"`
	Name                   string   `toml:"name"`
	Filters                []string `toml:"filters"`
	MemberCheck            bool     `toml:"member_check"`
	UseCookiesForDownloads bool     `toml:"use_cookies_for_downloads"`
}

// --- YouTube Specific ---

type YTConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval           string   `toml:"poll_interval"`
		IgnoreOlderThan        string   `toml:"ignore_older_than"`
		MaxRequestsPerSecond   float64  `toml:"max_requests_per_second"`
		CheckMethod            string   `toml:"check_method"`
		FallbackDuration       string   `toml:"fallback_duration"`
		CookiesFile            string   `toml:"cookies_file"`
		UseCookiesForDownloads bool     `toml:"use_cookies_for_downloads"`
		MemberCheckAll         bool     `toml:"member_check_all"`
		MemberCheckArgs        []string `toml:"member_check_args"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}

// --- Twitch Specific ---

type TwitchConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval         string  `toml:"poll_interval"`
		MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}

// --- Loaders ---

type ConfigWarning struct {
	Path    string
	Key     string
	Message string
}

func (w ConfigWarning) String() string {
	return fmt.Sprintf("%s: %s", w.Path, w.Message)
}

func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	cfg, _, err := LoadGlobalConfigWithWarnings(path)
	return cfg, err
}

func LoadGlobalConfigWithWarnings(path string) (*GlobalConfig, []ConfigWarning, error) {
	cfg := GetDefaultGlobalConfig()
	defaults := GetDefaultGlobalConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectGlobalConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

func LoadYTConfig(path string) (*YTConfig, error) {
	cfg, _, err := LoadYTConfigWithWarnings(path)
	return cfg, err
}

func LoadYTConfigWithWarnings(path string) (*YTConfig, []ConfigWarning, error) {
	cfg := GetDefaultYTConfig()
	defaults := GetDefaultYTConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectYTConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

func LoadTwitchConfig(path string) (*TwitchConfig, error) {
	cfg, _, err := LoadTwitchConfigWithWarnings(path)
	return cfg, err
}

func LoadTwitchConfigWithWarnings(path string) (*TwitchConfig, []ConfigWarning, error) {
	cfg := GetDefaultTwitchConfig()
	defaults := GetDefaultTwitchConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectTwitchConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

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
	addMissingWarning(&warnings, path, meta, []string{"scraper", "poll_interval"}, defaults.Scraper.PollInterval)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "ignore_older_than"}, defaults.Scraper.IgnoreOlderThan)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "max_requests_per_second"}, defaults.Scraper.MaxRequestsPerSecond)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "check_method"}, defaults.Scraper.CheckMethod)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "fallback_duration"}, defaults.Scraper.FallbackDuration)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "cookies_file"}, defaults.Scraper.CookiesFile)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "use_cookies_for_downloads"}, defaults.Scraper.UseCookiesForDownloads)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "member_check_all"}, defaults.Scraper.MemberCheckAll)
	addMissingWarning(&warnings, path, meta, []string{"scraper", "member_check_args"}, defaults.Scraper.MemberCheckArgs)

	if strings.TrimSpace(cfg.StreamMon.WorkingDirectory) == "" {
		addInvalidWarning(&warnings, path, "streammon.working_directory", cfg.StreamMon.WorkingDirectory, defaults.StreamMon.WorkingDirectory, "must not be empty")
		cfg.StreamMon.WorkingDirectory = defaults.StreamMon.WorkingDirectory
	}
	if len(cfg.StreamMon.Args) == 0 {
		addInvalidWarning(&warnings, path, "streammon.args", cfg.StreamMon.Args, defaults.StreamMon.Args, "must include downloader arguments")
		cfg.StreamMon.Args = defaults.StreamMon.Args
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
	if usesYouTubeCookies(cfg) && strings.TrimSpace(cfg.Scraper.CookiesFile) == "" {
		addInvalidWarning(&warnings, path, "scraper.cookies_file", cfg.Scraper.CookiesFile, defaults.Scraper.CookiesFile, "must not be empty when cookie-backed checks or downloads are enabled")
		cfg.Scraper.CookiesFile = defaults.Scraper.CookiesFile
	}
	addChannelWarnings(&warnings, path, cfg.Channels)

	addUndecodedWarnings(&warnings, path, meta)
	return warnings
}

func usesYouTubeCookies(cfg *YTConfig) bool {
	if cfg.Scraper.UseCookiesForDownloads || cfg.Scraper.MemberCheckAll {
		return true
	}
	for _, ch := range cfg.Channels {
		if ch.UseCookiesForDownloads || ch.MemberCheck {
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

func addMissingWarning(warnings *[]ConfigWarning, path string, meta toml.MetaData, key []string, defaultValue any) {
	if meta.IsDefined(key...) {
		return
	}

	keyName := strings.Join(key, ".")
	*warnings = append(*warnings, ConfigWarning{
		Path:    path,
		Key:     keyName,
		Message: fmt.Sprintf("missing %s; using default %s", keyName, formatDefault(defaultValue)),
	})
}

func addInvalidWarning(warnings *[]ConfigWarning, path, key string, value, defaultValue any, reason string) {
	*warnings = append(*warnings, ConfigWarning{
		Path:    path,
		Key:     key,
		Message: fmt.Sprintf("invalid %s=%s (%s); using default %s", key, formatDefault(value), reason, formatDefault(defaultValue)),
	})
}

func addUndecodedWarnings(warnings *[]ConfigWarning, path string, meta toml.MetaData) {
	for _, key := range meta.Undecoded() {
		keyName := key.String()
		*warnings = append(*warnings, ConfigWarning{
			Path:    path,
			Key:     keyName,
			Message: fmt.Sprintf("unknown %s; value is ignored", keyName),
		})
	}
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

func validTimezone(timezone string) bool {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return false
	}
	if _, err := time.LoadLocation(timezone); err == nil {
		return true
	}
	if after, ok := strings.CutPrefix(timezone, "UTC"); ok {
		timezone = after
	}
	_, err := parseUTCOffset(timezone)
	return err == nil
}

func parseUTCOffset(offset string) (int, error) {
	offset = strings.TrimSpace(offset)
	if offset == "" {
		return 0, nil
	}

	sign := int64(1)
	if strings.HasPrefix(offset, "-") {
		sign = -1
		offset = offset[1:]
	} else if strings.HasPrefix(offset, "+") {
		offset = offset[1:]
	}

	parts := strings.Split(offset, ":")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid offset format")
	}

	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	minutes := int64(0)
	if len(parts) == 2 {
		minutes, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, err
		}
	}

	if hours > 23 || minutes > 59 {
		return 0, fmt.Errorf("offset out of range")
	}

	return int(sign * (hours*3600 + minutes*60)), nil
}

func formatDefault(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []string:
		quoted := make([]string, 0, len(v))
		for _, item := range v {
			quoted = append(quoted, fmt.Sprintf("%q", item))
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}
