package config

import "github.com/BurntSushi/toml"

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
	ID      string   `toml:"id"`
	Name    string   `toml:"name"`
	Filters []string `toml:"filters"`
}

// --- YouTube Specific ---

type YTConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval         string  `toml:"poll_interval"`
		IgnoreOlderThan      string  `toml:"ignore_older_than"`
		MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
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

func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	cfg := GetDefaultGlobalConfig()
	_, err := toml.DecodeFile(path, cfg)
	return cfg, err
}

func LoadYTConfig(path string) (*YTConfig, error) {
	cfg := GetDefaultYTConfig()
	_, err := toml.DecodeFile(path, cfg)
	return cfg, err
}

func LoadTwitchConfig(path string) (*TwitchConfig, error) {
	cfg := GetDefaultTwitchConfig()
	_, err := toml.DecodeFile(path, cfg)
	return cfg, err
}
