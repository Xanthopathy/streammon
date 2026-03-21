package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// launchDownloader creates a lockfile and starts the downloader subprocess.
// This function must be called with the downloadMutex held.
// It returns true on success, false on failure.
func (b *BaseMonitor) launchDownloader(ch config.Channel, status LiveInfo, lockPath string) bool {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	// Create synchronization for waiting state detection
	isWaiting := &atomic.Bool{}

	// Callback to detect waiting state from subprocess output
	outputCallback := func(line string) {
		if strings.Contains(line, "[retry-streams]") {
			isWaiting.Store(true)
		} else if strings.Contains(line, "frame=") || strings.Contains(line, "[download]") {
			// If we see active download progress, we are no longer waiting.
			isWaiting.Store(false)
		}
	}

	// Create lockfile
	if err := util.CreateLock(lockPath); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating lockfile for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return false
	}

	// Build command using the controller
	cmd := b.controller.BuildDownloaderCmd(ch, status)

	// Build command string for logging
	commandStr := cmd.Path
	if len(cmd.Args) > 1 {
		commandStr += " " + util.JoinCommandArgs(cmd.Args[1:])
	}

	// Create channel specific directory
	channelDir := filepath.Join(streamMonCfg.WorkingDirectory, util.SanitizeFolderName(ch.Name))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating directory for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return false
	}
	cmd.Dir = channelDir

	// Determine which debug flags to enable based on platform and config
	apiDebug := false
	dlpDebug := false

	switch logPrefix {
	case "Twitch":
		apiDebug = globalCfg.TwitchAPIVerboseDebug
		dlpDebug = globalCfg.TwitchDlpVerboseDebug
	case "YT":
		apiDebug = globalCfg.YoutubeVerboseDebug
		dlpDebug = globalCfg.YoutubeDlpVerboseDebug
	}

	logger, err := util.NewDownloadLogger(
		channelDir,
		ch.ID,
		ch.Name,
		status.VideoID,
		status.CreatedAt,
		globalCfg,
		logPrefix,
		logColor,
		apiDebug,
		dlpDebug,
		commandStr,
	)
	if err != nil {
		fmt.Printf("%s [%s%s%s] Error creating logger for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return false
	}

	// Confirm dlpDebug setting
	if dlpDebug {
		logger.LogRegular("Raw subprocess output will be shown (dlp_verbose_debug=true)")
	}

	// Force colors in subprocess output (yt-dlp, twitch-dlp)
	// Set environment variables to enable color output even when piping
	// Doesn't work, twitch-dlp already does this and yt-dlp doesn't show color with this
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1", "TERM=xterm-256color")

	// Setup subprocess output redirection
	// Pipe output if we need to log it or show it in terminal (dlpDebug)
	// Determine debugType based on platform prefix
	var debugType string
	switch logPrefix {
	case "YT":
		debugType = "yt-dlp"
	case "Twitch":
		debugType = "twitch-dlp"
	default:
		debugType = "dlp"
	}

	if globalCfg.SaveDownloadLogs || dlpDebug {
		stdoutPipe, errOut := cmd.StdoutPipe()
		stderrPipe, errErr := cmd.StderrPipe()

		if errOut == nil && stdoutPipe != nil {
			go util.ReadPipeAndLog(stdoutPipe, logger, debugType, outputCallback)
		}
		if errErr == nil && stderrPipe != nil {
			go util.ReadPipeAndLog(stderrPipe, logger, debugType, outputCallback)
		}
	}

	// Log the command if dlp debug is enabled (for terminal display)
	if dlpDebug {
		logger.LogSubprocessOutput("COMMAND: "+commandStr, debugType)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		logger.LogError(fmt.Sprintf("Error starting download for %s: %v", ch.Name, err))
		util.DeleteLock(lockPath) // Clean up lock on failure
		logger.Close()
		return false
	}

	logger.LogRegular(fmt.Sprintf("%sStarted download for %s%s: %s", util.ColorGreen, ch.Name, util.ColorReset, status.Title))

	// Store process info
	proc := &downloadProcess{
		cmd:       cmd,
		videoID:   status.VideoID,
		lockPath:  lockPath,
		logger:    logger,
		isWaiting: isWaiting,
	}
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
	return true
}

// waitForDownload blocks until a download process finishes, then cleans up.
func (b *BaseMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Reset terminal title once subprocess completes
	util.SetTerminalTitle("streammon")

	// IMPORTANT: Release the download slot first thing after the process exits.
	<-downloadSlots

	// Now clean up other resources.
	b.downloadMutex.Lock()
	delete(b.activeDownloads, ch.ID)
	b.downloadMutex.Unlock()

	util.DeleteLock(proc.lockPath)
	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()

	util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("Released download slot for %s. Slots used: %d/%d.", ch.Name, len(downloadSlots), cap(downloadSlots)))

	if err != nil {
		proc.logger.LogError(fmt.Sprintf("Download for %s finished with error: %v", ch.Name, err))
	} else {
		proc.logger.LogRegular(fmt.Sprintf("Download for %s finished successfully.", ch.Name))

		// Mark this video as downloaded in the session cache
		b.downloadedVidMu.Lock()
		if _, ok := b.downloadedVideos[ch.ID]; !ok {
			b.downloadedVideos[ch.ID] = make(map[string]bool)
		}
		b.downloadedVideos[ch.ID][proc.videoID] = true
		b.downloadedVidMu.Unlock()

		// Archive the downloaded video ID if enabled
		shouldArchive := false
		if logPrefix == "YT" && globalCfg.YoutubeArchiveDownloads {
			shouldArchive = true
		} else if logPrefix == "Twitch" && globalCfg.TwitchArchiveDownloads {
			shouldArchive = true
		}

		if shouldArchive {
			archivePath := filepath.Join(b.controller.GetStreamMonConfig().WorkingDirectory, "archive.txt")
			if err := util.AppendLineToFile(archivePath, proc.videoID); err != nil {
				proc.logger.LogError(fmt.Sprintf("Failed to archive video ID: %v", err))
			} else {
				b.archivedVidMu.Lock()
				b.archivedVideos[proc.videoID] = true
				b.archivedVidMu.Unlock()
			}
		}
	}

	proc.logger.Close()
}
