package config

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

type YTConfig struct {
	StreamMon    StreamMonConfig `toml:"streammon"`
	LivestreamDL struct {
		Enabled    bool     `toml:"enabled"`
		UseCookies bool     `toml:"use_cookies"`
		Args       []string `toml:"args"`
	} `toml:"livestream_dl"`
	Scraper struct {
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

type TwitchConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		PollInterval         string  `toml:"poll_interval"`
		MaxRequestsPerSecond float64 `toml:"max_requests_per_second"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}
