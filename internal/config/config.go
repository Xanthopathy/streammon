package config

import "github.com/BurntSushi/toml"

// --- Shared Structures ---

type GlobalConfig struct {
	Timezone                   string `toml:"timezone"`
	MaxConcurrentDownloads     int    `toml:"max_concurrent_downloads"`
	EnableYoutube              bool   `toml:"enable_youtube"`
	EnableTwitch               bool   `toml:"enable_twitch"`
	SaveDownloadLogs           bool   `toml:"save_download_logs"`
	YoutubeArchiveDownloads    bool   `toml:"youtube_archive_downloads"`
	TwitchArchiveDownloads     bool   `toml:"twitch_archive_downloads"`
	SubprocessProgressInterval int    `toml:"subprocess_progress_interval"`
	SubprocessWaitInterval     int    `toml:"subprocess_wait_interval"`
	YoutubeVerboseDebug        bool   `toml:"youtube_verbose_debug"`
	YoutubeAPIVerboseDebug     bool   `toml:"youtube_api_verbose_debug"`
	YoutubeDlpVerboseDebug     bool   `toml:"youtube_dlp_verbose_debug"`
	TwitchVerboseDebug         bool   `toml:"twitch_verbose_debug"`
	TwitchAPIVerboseDebug      bool   `toml:"twitch_api_verbose_debug"`
	TwitchDlpVerboseDebug      bool   `toml:"twitch_dlp_verbose_debug"`
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
	Timezone  string          `toml:"timezone"` // Optional: override global timezone for YouTube logs
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval        string  `toml:"poll_interval"`
		IgnoreOlderThan     string  `toml:"ignore_older_than"`
		MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}

// --- Twitch Specific ---

type TwitchConfig struct {
	Timezone  string          `toml:"timezone"` // Optional: override global timezone for Twitch logs
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval         string  `toml:"poll_interval"`
		MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}

// --- Loaders ---

func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	var cfg GlobalConfig
	_, err := toml.DecodeFile(path, &cfg)
	return &cfg, err
}

func LoadYTConfig(path string) (*YTConfig, error) {
	var cfg YTConfig
	_, err := toml.DecodeFile(path, &cfg)
	return &cfg, err
}

func LoadTwitchConfig(path string) (*TwitchConfig, error) {
	var cfg TwitchConfig
	_, err := toml.DecodeFile(path, &cfg)
	return &cfg, err
}
