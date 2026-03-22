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
	return &YTConfig{
		StreamMon: StreamMonConfig{
			WorkingDirectory: "download_yt",
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
			PollInterval         string  `toml:"poll_interval"`
			IgnoreOlderThan      string  `toml:"ignore_older_than"`
			MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
			CheckMethod          string  `toml:"check_method"`
			FallbackDuration     string  `toml:"fallback_duration"`
		}{
			PollInterval:         "60s",
			IgnoreOlderThan:      "24h",
			MaxRequestsPerSecond: 2,
			CheckMethod:          "rss",
			FallbackDuration:     "15m",
		},
	}
}

// GetDefaultTwitchConfig returns the default configuration for the Twitch monitor.
func GetDefaultTwitchConfig() *TwitchConfig {
	return &TwitchConfig{
		StreamMon: StreamMonConfig{
			WorkingDirectory: "download_twitch",
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
