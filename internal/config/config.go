package config

import "github.com/BurntSushi/toml"

// --- Shared Structures ---

type GlobalConfig struct {
	Timezone               string `toml:"timezone"`
	MaxConcurrentDownloads int    `toml:"max_concurrent_downloads"`
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

type RSSConfig struct {
	PollInterval    string `toml:"poll_interval"`
	IgnoreOlderThan string `toml:"ignore_older_than"`
}

type YTConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Scraper   struct {
		RSS RSSConfig `toml:"rss"`
	} `toml:"scraper"`
	Channels []Channel `toml:"channel"`
}

// --- Twitch Specific ---

type TwitchConfig struct {
	StreamMon StreamMonConfig `toml:"streammon"`
	Channels  []Channel       `toml:"channel"`
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
