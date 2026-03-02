package main

import (
	"fmt"
	"sync"

	"streammon/internal/config"
	"streammon/internal/monitor"
	"streammon/internal/util"
)

func main() {
	util.PrintBanner()

	// 1. Load Configuration
	fmt.Println("[INFO] Loading configurations...")

	ytCfg, err := config.LoadYTConfig("configs/config_yt.toml")
	if err != nil {
		fmt.Printf("%s[WARN] Could not load config_yt.toml: %v. YouTube monitor will not run.%s\n", util.ColorYellow, err, util.ColorReset)
		ytCfg = nil // Ensure it's nil
	}

	twitchCfg, err := config.LoadTwitchConfig("configs/config_twitch.toml")
	if err != nil {
		fmt.Printf("%s[WARN] Could not load config_twitch.toml: %v. Twitch monitor will not run.%s\n", util.ColorYellow, err, util.ColorReset)
		twitchCfg = nil // Ensure it's nil
	}

	if ytCfg == nil && twitchCfg == nil {
		fmt.Printf("%s[FATAL] No valid configuration files found. Exiting.%s\n", util.ColorRed, util.ColorReset)
		return
	}

	// 2. Start Monitors
	var wg sync.WaitGroup

	if ytCfg != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.MonitorYouTube(ytCfg)
		}()
	}

	if twitchCfg != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.MonitorTwitch(twitchCfg)
		}()
	}

	// Keep main thread alive until all goroutines are done
	wg.Wait()
	fmt.Println("[INFO] All monitors have finished.")
}
