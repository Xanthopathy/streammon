package main

import (
	"fmt"
	"sync"

	"streammon/internal/config"
	"streammon/internal/monitor"
	"streammon/internal/util"
)

func main() {
	util.SetTerminalTitle("streammon")
	util.PrintBanner()

	// Bootstrap logger with default settings for startup messages
	// We use a dummy config initially, defaulting to UTC
	defaultCfg := &config.GlobalConfig{Timezone: "UTC"}
	sysLogger := util.NewLogger(defaultCfg, "System", util.ColorBlue)

	// 1. Load Configuration
	sysLogger.LogRegular("Loading configurations...")

	globalCfg, err := config.LoadGlobalConfig("streammon_config.toml")
	if err != nil {
		globalCfg, err = config.LoadGlobalConfig("configs/streammon_config.toml")
	}
	if err != nil {
		sysLogger.Warn(fmt.Sprintf("Could not load streammon_config.toml: %v. Using defaults (UTC).", err))
		globalCfg = &config.GlobalConfig{
			Timezone:                   "UTC",
			MaxConcurrentDownloads:     10,
			EnableYoutube:              true,
			EnableTwitch:               true,
			SaveDownloadLogs:           true,
			YoutubeArchiveDownloads:    true,
			TwitchArchiveDownloads:     true,
			SubprocessProgressInterval: 10,
			SubprocessWaitInterval:     60,
			YoutubeVerboseDebug:        true,
			TwitchVerboseDebug:         true,
		}
	}

	// Update logger with loaded config (for correct timezone)
	sysLogger = util.NewLogger(globalCfg, "System", util.ColorBlue)

	var ytCfg *config.YTConfig
	if globalCfg.EnableYoutube {
		var err error
		ytCfg, err = config.LoadYTConfig("streammon_config_yt.toml")
		if err != nil {
			ytCfg, err = config.LoadYTConfig("configs/streammon_config_yt.toml")
		}
		if err != nil {
			sysLogger.Warn(fmt.Sprintf("YouTube is enabled, but could not load streammon_config_yt.toml: %v. YouTube monitor will not run.", err))
			ytCfg = nil // Ensure it's nil
		}
	}

	var twitchCfg *config.TwitchConfig
	if globalCfg.EnableTwitch {
		var err error
		twitchCfg, err = config.LoadTwitchConfig("streammon_config_twitch.toml")
		if err != nil {
			twitchCfg, err = config.LoadTwitchConfig("configs/streammon_config_twitch.toml")
		}
		if err != nil {
			sysLogger.Warn(fmt.Sprintf("Twitch is enabled, but could not load streammon_config_twitch.toml: %v. Twitch monitor will not run.", err))
			twitchCfg = nil // Ensure it's nil
		}
	}

	if ytCfg == nil && twitchCfg == nil {
		sysLogger.LogError("No monitors are enabled or correctly configured. Exiting.")
		return
	}

	// 2. Start Monitors
	var wg sync.WaitGroup

	if ytCfg != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.MonitorYouTube(ytCfg, globalCfg)
		}()
	}

	if twitchCfg != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.MonitorTwitch(twitchCfg, globalCfg)
		}()
	}

	// Keep main thread alive until all goroutines are done
	wg.Wait()
	sysLogger.LogRegular("All monitors have finished.")
}
