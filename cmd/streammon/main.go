package main

import (
	"fmt"
	"sync"

	"streammon/internal/config"
	"streammon/internal/monitor"
	"streammon/internal/util"
)

func main() {
	util.SetTerminalTitle("StreamMon")
	util.PrintBanner()

	// 1. Load Configuration
	fmt.Printf("[%sINFO%s] Loading configurations...\n", util.ColorBlue, util.ColorReset)

	globalCfg, err := config.LoadGlobalConfig("streammon_config.toml")
	if err != nil {
		globalCfg, err = config.LoadGlobalConfig("configs/streammon_config.toml")
	}
	if err != nil {
		fmt.Printf("[%sWARN%s] Could not load streammon_config.toml: %v. Using defaults (UTC).\n", util.ColorYellow, err, util.ColorReset)
		globalCfg = &config.GlobalConfig{
			Timezone:               "UTC",
			MaxConcurrentDownloads: 10,
			EnableYoutube:          true,
			EnableTwitch:           true,
			YoutubeVerboseDebug:    false,
			TwitchVerboseDebug:     false,
		}
	}

	var ytCfg *config.YTConfig
	if globalCfg.EnableYoutube {
		var err error
		ytCfg, err = config.LoadYTConfig("streammon_config_yt.toml")
		if err != nil {
			ytCfg, err = config.LoadYTConfig("configs/streammon_config_yt.toml")
		}
		if err != nil {
			fmt.Printf("[%sWARN%s] YouTube is enabled, but could not load streammon_config_yt.toml: %v. YouTube monitor will not run.\n", util.ColorYellow, err, util.ColorReset)
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
			fmt.Printf("[%sWARN%s] Twitch is enabled, but could not load streammon_config_twitch.toml: %v. Twitch monitor will not run.\n", util.ColorYellow, err, util.ColorReset)
			twitchCfg = nil // Ensure it's nil
		}
	}

	if ytCfg == nil && twitchCfg == nil {
		fmt.Printf("%s[FATAL] No monitors are enabled or correctly configured. Exiting.%s\n", util.ColorRed, util.ColorReset)
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
	fmt.Println("[INFO] All monitors have finished.")
}
