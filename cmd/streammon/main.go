package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"streammon/internal/config"
	"streammon/internal/monitor"
	"streammon/internal/util/ansi"
	"streammon/internal/util/lockfile"
	"streammon/internal/util/logging"
	"streammon/internal/util/terminal"
	"streammon/internal/util/updatecheck"
)

var currentVersion = "dev" // git tag v1.x.x then run build

func main() {
	terminal.SetTerminalTitle("streammon")
	terminal.PrintBanner()

	// Get executable directory for config loading
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	// Bootstrap logger with default settings for startup messages
	// We use a dummy config initially, defaulting to UTC
	defaultCfg := config.GetDefaultGlobalConfig()
	sysLogger := logging.NewLogger(defaultCfg, "System", ansi.ColorCyan)

	sysLogger.Logf("streammon version %s", currentVersion)

	// 1. Load Configuration
	sysLogger.LogRegular("Loading configurations...")

	globalCfg, globalWarnings, err := config.LoadGlobalConfigWithWarnings(filepath.Join(exeDir, "streammon_config.toml"))
	if err != nil {
		globalCfg, globalWarnings, err = config.LoadGlobalConfigWithWarnings("streammon_config.toml")
		if err != nil {
			globalCfg, globalWarnings, err = config.LoadGlobalConfigWithWarnings("configs/streammon_config.toml")
		}
	}
	if err != nil {
		sysLogger.Warn(fmt.Sprintf("Could not load streammon_config.toml: %v. Using defaults (UTC).", err))
		globalCfg = config.GetDefaultGlobalConfig()
	}

	// Update logger with loaded config (for correct timezone)
	sysLogger = logging.NewLogger(globalCfg, "System", ansi.ColorCyan)
	logConfigWarnings(sysLogger, globalWarnings)

	// Start update check in the background
	go func() {
		updateMsg, err := updatecheck.CheckForUpdates(currentVersion)
		if err != nil {
			sysLogger.Warn(fmt.Sprintf("Failed to check for updates: %v", err))
		} else if updateMsg != "" {
			sysLogger.LogRegular(updateMsg)
		}
	}()

	var ytCfg *config.YTConfig
	if globalCfg.EnableYoutube {
		var err error
		var warnings []config.ConfigWarning
		ytCfg, warnings, err = config.LoadYTConfigWithWarnings(filepath.Join(exeDir, "streammon_config_yt.toml"))
		if err != nil {
			ytCfg, warnings, err = config.LoadYTConfigWithWarnings("streammon_config_yt.toml")
			if err != nil {
				ytCfg, warnings, err = config.LoadYTConfigWithWarnings("configs/streammon_config_yt.toml")
			}
		}
		if err != nil {
			sysLogger.Warn(fmt.Sprintf("YouTube is enabled, but could not load streammon_config_yt.toml: %v. YouTube monitor will not run.", err))
			ytCfg = nil // Ensure it's nil
		} else {
			logConfigWarnings(sysLogger, warnings)
		}
	}

	var twitchCfg *config.TwitchConfig
	if globalCfg.EnableTwitch {
		var err error
		var warnings []config.ConfigWarning
		twitchCfg, warnings, err = config.LoadTwitchConfigWithWarnings(filepath.Join(exeDir, "streammon_config_twitch.toml"))
		if err != nil {
			twitchCfg, warnings, err = config.LoadTwitchConfigWithWarnings("streammon_config_twitch.toml")
			if err != nil {
				twitchCfg, warnings, err = config.LoadTwitchConfigWithWarnings("configs/streammon_config_twitch.toml")
			}
		}
		if err != nil {
			sysLogger.Warn(fmt.Sprintf("Twitch is enabled, but could not load streammon_config_twitch.toml: %v. Twitch monitor will not run.", err))
			twitchCfg = nil // Ensure it's nil
		} else {
			logConfigWarnings(sysLogger, warnings)
		}
	}

	if ytCfg == nil && twitchCfg == nil {
		sysLogger.LogError("No monitors are enabled or correctly configured. Exiting.")
		return
	}

	// 2. Cleanup Lockfiles (if enabled)
	if globalCfg.ClearAllLockfiles {
		sysLogger.LogRegular("Cleaning up old lockfiles...")
		if ytCfg != nil {
			clearLockfiles(sysLogger, "YouTube", ytCfg.StreamMon.WorkingDirectory)
		}
		if twitchCfg != nil {
			clearLockfiles(sysLogger, "Twitch", twitchCfg.StreamMon.WorkingDirectory)
		}
	}

	// 3. Start Monitors
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

func logConfigWarnings(logger *logging.Logger, warnings []config.ConfigWarning) {
	for _, warning := range warnings {
		logger.Warn("Config: " + warning.String())
	}
}

func clearLockfiles(logger *logging.Logger, platformName, workingDirectory string) {
	count, err := lockfile.ClearLockfiles(workingDirectory)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn(fmt.Sprintf("Could not clear %s lockfiles from %s: %v", platformName, workingDirectory, err))
		}
		return
	}

	if count > 0 {
		logger.Logf("Removed %d old %s lockfile(s) from %s", count, platformName, workingDirectory)
	}
}
