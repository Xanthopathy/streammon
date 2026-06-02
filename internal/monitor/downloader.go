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
	"streammon/internal/util/terminal"
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
	shouldLogSlots := (logPrefix == "Twitch" && globalCfg.TwitchVerboseDebug) || (logPrefix == "YT" && globalCfg.YoutubeVerboseDebug)
	if shouldLogSlots {
		// Note: len(downloadSlots) shows the number of *active* slots.
		// Since we've already acquired one, the number of slots currently in use is len(downloadSlots).
		b.logger.Logf("Acquired download slot for %s%s%s. Slots used: %d/%d.",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

	// Create synchronization for waiting state detection
	isWaiting := &atomic.Bool{}

	// Create synchronization for merger detection
	mergerDetected := &atomic.Bool{}

	// Create synchronization for downloader-specific completion markers
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

	logger, err := logging.NewLoggerForDownload(
		channelDir,
		ch.ID,
		ch.Name,
		status.VideoID,
		globalCfg,
		logPrefix,
		logColor,
		apiDebug,
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

// waitForDownload blocks until a download process finishes, then cleans up.
func (b *BaseMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Give subprocess time to clean up residual files, temp files, and finalize disk writes
	// (yt-dlp and twitch-dlp may still be flushing data after process.Wait returns)
	time.Sleep(time.Second * 5)

	// Reset terminal title once subprocess completes
	terminal.SetTerminalTitle("streammon")

	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()

	// Extract exit code from the process
	exitCode := -1
	if proc.cmd.ProcessState != nil {
		exitCode = proc.cmd.ProcessState.ExitCode()
	}

	// Determine success using downloader-specific completion markers plus file existence.
	// yt-dlp can return non-zero after a successful merge, while twitch-dlp does not emit yt-dlp merger markers.
	outputFileExists := false
	mergerSuccess := proc.mergerDetected.Load()
	downloadComplete := proc.downloadCompleted.Load()

	// Check if output file exists in the working directory
	// The output file should match the pattern from the downloader command
	if proc.cmd.Dir != "" {
		files, err := os.ReadDir(proc.cmd.Dir)
		if err == nil {
			for _, file := range files {
				if !file.IsDir() {
					info, err := file.Info()
					if err != nil {
						continue
					}
					if mediaFileMatchesDownload(file.Name(), info.ModTime(), proc) {
						outputFileExists = true
						break
					}
				}
			}
		}
	}

	// Log exit code and diagnostic info
	if exitCode >= 0 {
		switch proc.downloaderName {
		case "yt-dlp":
			proc.logger.LogRegular(fmt.Sprintf("[%sDiagnostic%s] %s exit code: %d | merger_detected: %v | file_exists: %v", ansi.ColorBlue, ansi.ColorReset, proc.downloaderName, exitCode, mergerSuccess, outputFileExists))
		case "twitch-dlp":
			proc.logger.LogRegular(fmt.Sprintf("[%sDiagnostic%s] %s exit code: %d | completion_detected: %v | file_exists: %v", ansi.ColorBlue, ansi.ColorReset, proc.downloaderName, exitCode, downloadComplete, outputFileExists))
		default:
			proc.logger.LogRegular(fmt.Sprintf("[%sDiagnostic%s] %s exit code: %d | completion_detected: %v | merger_detected: %v | file_exists: %v", ansi.ColorBlue, ansi.ColorReset, proc.downloaderName, exitCode, downloadComplete, mergerSuccess, outputFileExists))
		}
	}

	// Determine final success status
	isSuccess := false
	if proc.forcedTermination.Load() {
		// Forced termination by monitor (stream went offline)
		proc.logger.LogRegular(fmt.Sprintf("Download for %s%s%s stopped by monitor (stream offline).", ansi.ColorOrange, ch.Name, ansi.ColorReset))
		isSuccess = true // Treat forced termination as success (meaningful data captured)
	} else if proc.downloaderName == "yt-dlp" && mergerSuccess && outputFileExists {
		// Both success conditions met
		proc.logger.LogRegular(fmt.Sprintf("Download for %s%s%s finished successfully.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
		cleanupYTDLPResidue(proc.cmd.Dir, proc, proc.logger)
		isSuccess = true
	} else if proc.downloaderName == "twitch-dlp" && outputFileExists && (downloadComplete || exitCode == 0) {
		// twitch-dlp does not emit yt-dlp merger markers; use its own completion markers and file output.
		proc.logger.LogRegular(fmt.Sprintf("Download for %s%s%s finished successfully.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
		isSuccess = true
	} else if proc.downloaderName == "livestream_dl" && outputFileExists && exitCode == 0 {
		proc.logger.LogRegular(fmt.Sprintf("Download for %s%s%s finished successfully with livestream_dl fallback.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
		isSuccess = true
	} else {
		// One or both success conditions failed
		failureReasons := []string{}
		switch proc.downloaderName {
		case "yt-dlp":
			if !mergerSuccess {
				failureReasons = append(failureReasons, "no_merger_detected")
			}
		case "twitch-dlp":
			if !downloadComplete && exitCode != 0 {
				failureReasons = append(failureReasons, "no_completion_detected")
			}
		default:
			if !downloadComplete && !mergerSuccess && exitCode != 0 {
				failureReasons = append(failureReasons, "no_completion_detected")
			}
		}
		if !outputFileExists {
			failureReasons = append(failureReasons, "output_file_not_found")
		}
		if proc.downloaderName == "yt-dlp" && b.startFallbackDownload(ch, proc) {
			go b.waitForDownload(ch, proc)
			return
		}
		proc.logger.LogError(fmt.Sprintf("Download for %s%s%s finished with error: %v (exit_code=%d, reasons=%v)",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, err, exitCode, failureReasons))
		isSuccess = false
	}

	// The full download lifecycle is complete. Release the shared slot and lockfile.
	<-downloadSlots
	lockfile.DeleteLock(proc.lockPath)
	proc.logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", proc.lockPath))

	shouldLogSlots := (logPrefix == "Twitch" && globalCfg.TwitchVerboseDebug) || (logPrefix == "YT" && globalCfg.YoutubeVerboseDebug)
	if shouldLogSlots {
		proc.logger.Logf("Released download slot for %s%s%s. Slots used: %d/%d.", ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

	// Finalize success or set pending state for YouTube
	if isSuccess {
		if logPrefix == "YT" && !proc.forcedTermination.Load() {
			b.setPendingYTSuccess(ch.ID, proc.videoID)
			proc.logger.LogRegular("Waiting for the next YT poll before archiving this download.")
		} else {
			b.finalizeSuccessfulDownload(ch.ID, proc.videoID, proc.logger)
		}
	}

	// Clean up active download entry
	b.downloadMutex.Lock()
	delete(b.activeDownloads, ch.ID)
	b.downloadMutex.Unlock()

	proc.logger.Close()
}

func (b *BaseMonitor) startFallbackDownload(ch config.Channel, proc *downloadProcess) bool {
	if proc.fallbackAttempted {
		return false
	}
	controller, ok := b.controller.(FallbackDownloaderController)
	if !ok {
		return false
	}

	cmd, downloaderName, enabled := controller.BuildFallbackDownloaderCmd(ch, proc.status)
	if !enabled || cmd == nil {
		return false
	}
	proc.fallbackAttempted = true

	cmd.Dir = proc.cmd.Dir
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1", "TERM=xterm-256color")
	if b.controller.GetGlobalConfig().SaveDownloadLogs || b.controller.GetGlobalConfig().YoutubeDlpVerboseDebug {
		if stdoutPipe, err := cmd.StdoutPipe(); err == nil && stdoutPipe != nil {
			go logging.ReadPipeAndLog(stdoutPipe, proc.logger, downloaderName, proc.outputCallback)
		}
		if stderrPipe, err := cmd.StderrPipe(); err == nil && stderrPipe != nil {
			go logging.ReadPipeAndLog(stderrPipe, proc.logger, downloaderName, proc.outputCallback)
		}
	}

	commandStr := cmd.Path
	if len(cmd.Args) > 1 {
		commandStr += " " + text.JoinCommandArgs(cmd.Args[1:])
	}
	proc.logger.LogRegular(fmt.Sprintf("yt-dlp failed for %s%s%s. Trying livestream_dl fallback.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
	proc.logger.LogSubprocessOutput("COMMAND: "+commandStr, downloaderName)

	if err := cmd.Start(); err != nil {
		proc.logger.LogError(fmt.Sprintf("Error starting livestream_dl fallback for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
		return false
	}

	proc.cmd = cmd
	proc.downloaderName = downloaderName
	proc.startedAt = time.Now()
	proc.mergerDetected.Store(false)
	proc.downloadCompleted.Store(false)
	return true
}
