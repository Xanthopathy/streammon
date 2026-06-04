package config

// GetDefaultGlobalConfig returns the hardcoded default configuration.
// This provides a single source of truth for defaults used when loading fails or for initialization.
func GetDefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Timezone:                   "UTC",
		MaxConcurrentDownloads:     10,
		EnableYoutube:              true,
		EnableTwitch:               true,
		YoutubeVerboseDebug:        true,
		TwitchVerboseDebug:         true,
		TwitchAPIVerboseDebug:      false,
		TwitchDlpVerboseDebug:      true,
		YoutubeAPIVerboseDebug:     false,
		YoutubeDlpVerboseDebug:     true,
		YoutubeArchiveDownloads:    true,
		TwitchArchiveDownloads:     true,
		SaveDownloadLogs:           true,
		SubprocessProgressInterval: 30,
		SubprocessWaitInterval:     600,
		ClearAllLockfiles:          true,
	}
}

// GetDefaultYTConfig returns the default configuration for the YouTube monitor.
func GetDefaultYTConfig() *YTConfig {
	cfg := &YTConfig{
		StreamMon: StreamMonConfig{
			WorkingDirectory: "download_yt",
		},
		YTDLP: DownloaderConfig{
			Args: []string{
				"--wait-for-video", "60",
				"--live-from-start",
				"--js-runtime", "node",
				"--embed-thumbnail",
				"--convert-thumbnail", "png",
				"--write-thumbnail",
				"--retries", "10",
				"--output", "[%(upload_date)s] [%(id)s] [%(title)s] [%(channel)s].%(ext)s",
			},
		},
		Scraper: struct {
			PollInterval                             string   `toml:"poll_interval"`
			IgnoreOlderThan                          string   `toml:"ignore_older_than"`
			MaxRequestsPerSecond                     float64  `toml:"max_requests_per_second"`
			CheckMethod                              string   `toml:"check_method"`
			DownloaderMethod                         string   `toml:"downloader_method"`
			FallbackDuration                         string   `toml:"fallback_duration"`
			CookiesFile                              string   `toml:"cookies_file"`
			MemberCheckAll                           bool     `toml:"member_check_all"`
			MemberDownloader                         string   `toml:"member_downloader"`
			DownloadWaitRetries                      int      `toml:"download_wait_retries"`
			RetrySameDownloaderWithTimestampWhenLive bool     `toml:"retry_same_downloader_with_timestamp_when_live"`
			RetryOfflineWithoutLiveArgs              bool     `toml:"retry_offline_without_live_args"`
			MemberCheckArgs                          []string `toml:"member_check_args"`
		}{
			PollInterval:                             "60s",
			IgnoreOlderThan:                          "24h",
			MaxRequestsPerSecond:                     2,
			CheckMethod:                              "rss",
			DownloaderMethod:                         "yt-dlp",
			FallbackDuration:                         "15m",
			CookiesFile:                              "youtube_cookies.txt",
			MemberCheckAll:                           false,
			MemberDownloader:                         "livestream_dl",
			DownloadWaitRetries:                      3,
			RetrySameDownloaderWithTimestampWhenLive: false,
			RetryOfflineWithoutLiveArgs:              false,
			MemberCheckArgs: []string{
				"--flat-playlist",
				"--playlist-items", "1:3",
				"--dump-single-json",
				"--no-warnings",
			},
		},
	}
	cfg.LivestreamDL.Enabled = false
	cfg.LivestreamDL.Args = []string{
		"--resolution", "best",
		"--threads", "4",
		"--segment-retries", "10",
		"--output", "[%(upload_date)s] [%(id)s] [%(title)s] [%(channel)s]",
		"--write-thumbnail",
		"--embed-thumbnail",
		"--wait-for-video", "60",
		"--ytdlp-command-line-options=--js-runtime node",
	}
	return cfg
}

// GetDefaultTwitchConfig returns the default configuration for the Twitch monitor.
func GetDefaultTwitchConfig() *TwitchConfig {
	return &TwitchConfig{
		StreamMon: StreamMonConfig{
			WorkingDirectory: "download_twitch",
		},
		TwitchDLP: DownloaderConfig{
			Args: []string{
				"--live-from-start",
				"--retry-streams", "60",
				"--output", "[%(upload_date)s] [%(id)s] [%(uploader)s] [%(title)s].%(ext)s",
			},
		},
		Scraper: struct {
			PollInterval         string  `toml:"poll_interval"`
			MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
		}{
			PollInterval:         "30s",
			MaxRequestsPerSecond: 2,
		},
	}
}
