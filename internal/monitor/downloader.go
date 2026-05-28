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
	"streammon/internal/util/fileio"
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
		status.CreatedAt,
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

	// Start command
	if err := cmd.Start(); err != nil {
		logger.LogError(fmt.Sprintf("Error starting download for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
		lockfile.DeleteLock(lockPath) // Clean up lock on failure
		logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", lockPath))
		logger.Close()
		return false
	}
	startedAt := time.Now()

	logger.LogRegular(fmt.Sprintf("%sStarted download for%s %s%s%s: %s", ansi.ColorGreen, ansi.ColorReset, ansi.ColorOrange, ch.Name, ansi.ColorReset, status.Title))

	// Store process info
	proc := &downloadProcess{
		cmd:               cmd,
		videoID:           status.VideoID,
		downloaderName:    debugType,
		startedAt:         startedAt,
		lockPath:          lockPath,
		logger:            logger,
		isWaiting:         isWaiting,
		mergerDetected:    mergerDetected,
		downloadCompleted: downloadCompleted,
	}
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
	return true
}

func isMediaFile(name string) bool {
	return strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mkv") || strings.HasSuffix(name, ".webm")
}

func mediaFileMatchesDownload(name string, modTime time.Time, proc *downloadProcess) bool {
	if !isMediaFile(name) {
		return false
	}

	switch proc.downloaderName {
	case "yt-dlp":
		if strings.Contains(name, proc.videoID) {
			return true
		}
		if len(proc.videoID) >= 8 && strings.Contains(name, proc.videoID[:8]) {
			return true
		}
		return false
	case "twitch-dlp":
		// twitch-dlp's %(id)s can be a VOD-style ID (e.g. v2782168798), while
		// streammon tracks the live GQL stream ID. Use files touched by this run.
		return !modTime.Before(proc.startedAt.Add(-10 * time.Second))
	default:
		return strings.Contains(name, proc.videoID) || !modTime.Before(proc.startedAt.Add(-10*time.Second))
	}
}

// waitForDownload blocks until a download process finishes, then cleans up.
func (b *BaseMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Give subprocess time to clean up residual files, temp files, and finalize disk writes
	// (yt-dlp and twitch-dlp may still be flushing data after process.Wait returns)
	time.Sleep(time.Second * 5)

	// Reset terminal title once subprocess completes
	terminal.SetTerminalTitle("streammon")

	// IMPORTANT: Release the download slot first thing after the process exits.
	<-downloadSlots

	// Now clean up other resources.
	b.downloadMutex.Lock()
	delete(b.activeDownloads, ch.ID)
	b.downloadMutex.Unlock()

	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()
	lockfile.DeleteLock(proc.lockPath)
	proc.logger.LogEvent("LOCK", fmt.Sprintf("Deleted: %s", proc.lockPath))

	// Log slot release with correct styling (fixes double tag issue and enables for YT)
	shouldLogSlots := (logPrefix == "Twitch" && globalCfg.TwitchVerboseDebug) || (logPrefix == "YT" && globalCfg.YoutubeVerboseDebug)
	if shouldLogSlots {
		proc.logger.Logf("Released download slot for %s%s%s. Slots used: %d/%d.", ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

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
		isSuccess = true
	} else if proc.downloaderName == "twitch-dlp" && outputFileExists && (downloadComplete || exitCode == 0) {
		// twitch-dlp does not emit yt-dlp merger markers; use its own completion markers and file output.
		proc.logger.LogRegular(fmt.Sprintf("Download for %s%s%s finished successfully.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
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
		proc.logger.LogError(fmt.Sprintf("Download for %s%s%s finished with error: %v (exit_code=%d, reasons=%v)",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, err, exitCode, failureReasons))
		isSuccess = false
	}

	// Archive if success OR forced termination (assuming meaningful data was captured)
	if isSuccess {
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
			if err := fileio.AppendLineToFile(archivePath, proc.videoID); err != nil {
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
