package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/ansi"
	"streammon/internal/util/lockfile"
	"streammon/internal/util/logging"
	"streammon/internal/util/text"
)

// launchDownloader creates a lockfile and starts the downloader subprocess.
// This function must be called with the downloadMutex held.
// It returns true on success, false on failure.
func (b *BaseMonitor) launchDownloader(ch config.Channel, status models.LiveInfo, lockPath string) bool {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	// Log slot acquisition
	shouldLogSlots := (logPrefix == logPrefixTwitch && globalCfg.TwitchVerboseDebug) || (logPrefix == logPrefixYouTube && globalCfg.YoutubeVerboseDebug)
	if shouldLogSlots {
		// Note: len(downloadSlots) shows the number of *active* slots.
		// Since we've already acquired one, the number of slots currently in use is len(downloadSlots).
		b.logger.Logf("Acquired download slot for %s%s%s. Slots used: %d/%d.",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

	isWaiting := &atomic.Bool{}

	mergerDetected := &atomic.Bool{}

	downloadCompleted := &atomic.Bool{}

	// Callback to detect waiting state and completion markers from subprocess output
	outputCallback := func(line string) {
		if strings.Contains(line, "[retry-streams]") {
			isWaiting.Store(true)
		} else if strings.Contains(line, "frame=") || strings.Contains(line, "[download]") {
			// If we see active download progress, we are no longer waiting.
			isWaiting.Store(false)
		}

		// Track successful completion markers from yt-dlp post-processing.
		if strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats") {
			mergerDetected.Store(true)
		}

		// Track completion markers commonly emitted by twitch-dlp/ffmpeg.
		if strings.Contains(line, "[stats] Fragments") ||
			(strings.Contains(line, "frame=") && strings.Contains(line, "Lsize=")) ||
			(strings.Contains(line, "[out#") && strings.Contains(line, "muxing overhead:")) {
			downloadCompleted.Store(true)
		}
	}

	// Create lockfile
	if err := lockfile.CreateLock(lockPath); err != nil {
		b.logger.LogErrorf("Error creating lockfile for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err)
		return false
	}
	b.logger.LogEvent("LOCK", fmt.Sprintf("Created: %s", lockPath))

	// Build command using the controller
	cmd := b.controller.BuildDownloaderCmd(ch, status)

	// Build command string for logging
	commandStr := cmd.Path
	if len(cmd.Args) > 1 {
		commandStr += " " + text.JoinCommandArgs(cmd.Args[1:])
	}

	// Create channel specific directory
	channelDir := filepath.Join(streamMonCfg.WorkingDirectory, text.SanitizeFolderName(ch.Name))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		b.logger.LogErrorf("Error creating directory for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err)
		lockfile.DeleteLock(lockPath)
		b.logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", lockPath))
		return false
	}
	cmd.Dir = channelDir

	// Determine which subprocess debug flag to enable based on platform and config.
	dlpDebug := false

	switch logPrefix {
	case logPrefixTwitch:
		dlpDebug = globalCfg.TwitchDlpVerboseDebug
	case logPrefixYouTube:
		dlpDebug = globalCfg.YoutubeDlpVerboseDebug
	}

	logger, err := logging.NewLoggerForDownload(
		channelDir,
		ch.Name,
		status.VideoID,
		globalCfg,
		logPrefix,
		logColor,
		dlpDebug,
		commandStr,
	)
	if err != nil {
		b.logger.LogErrorf("Error creating logger for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err)
		lockfile.DeleteLock(lockPath)
		b.logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", lockPath))
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
	case logPrefixYouTube:
		debugType = "yt-dlp"
	case logPrefixTwitch:
		debugType = "twitch-dlp"
	default:
		debugType = "dlp"
	}

	if globalCfg.SaveDownloadLogs || dlpDebug {
		stdoutPipe, errOut := cmd.StdoutPipe()
		stderrPipe, errErr := cmd.StderrPipe()

		if errOut == nil && stdoutPipe != nil {
			go logging.ReadPipeAndLog(stdoutPipe, logger, debugType, outputCallback)
		}
		if errErr == nil && stderrPipe != nil {
			go logging.ReadPipeAndLog(stderrPipe, logger, debugType, outputCallback)
		}
	}

	// Log the command if dlp debug is enabled (for terminal display)
	if dlpDebug {
		logger.LogSubprocessOutput("COMMAND: "+commandStr, debugType)
	}

	proc := &downloadProcess{
		cmd:               cmd,
		videoID:           status.VideoID,
		downloaderName:    debugType,
		lockPath:          lockPath,
		logger:            logger,
		isWaiting:         isWaiting,
		mergerDetected:    mergerDetected,
		downloadCompleted: downloadCompleted,
		status:            status,
		outputCallback:    outputCallback,
	}

	// Start command
	if err := cmd.Start(); err != nil {
		logger.LogError(fmt.Sprintf("Error starting download for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
		if debugType == "yt-dlp" && b.startFallbackDownload(ch, proc) {
			b.activeDownloads[ch.ID] = proc
			go b.waitForDownload(ch, proc)
			return true
		}
		lockfile.DeleteLock(lockPath) // Clean up lock on failure
		logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", lockPath))
		logger.Close()
		return false
	}
	startedAt := time.Now()

	logger.LogRegular(fmt.Sprintf("%sStarted download for%s %s%s%s: %s", ansi.ColorGreen, ansi.ColorReset, ansi.ColorOrange, ch.Name, ansi.ColorReset, status.Title))

	// Store process info
	proc.startedAt = startedAt
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
	return true
}
