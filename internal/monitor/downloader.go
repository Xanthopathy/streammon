package monitor

import (
	"fmt"
	"os"
	"os/exec"
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
		b.logger.LogEventf("SLOT", "Acquired download slot for %s%s%s. Slots used: %d/%d.",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

	isWaiting := &atomic.Bool{}

	mergerDetected := &atomic.Bool{}

	downloadCompleted := &atomic.Bool{}
	postprocessFailed := &atomic.Bool{}
	fragmentFailure := &atomic.Bool{}
	extractorFailed := &atomic.Bool{}
	authFailure := &atomic.Bool{}
	diskFailure := &atomic.Bool{}
	processCrashed := &atomic.Bool{}
	hadDownloadActivity := &atomic.Bool{}
	downloadWaitRetries := 0
	if controller, ok := b.controller.(interface{ GetDownloadWaitRetries() int }); ok {
		downloadWaitRetries = controller.GetDownloadWaitRetries()
	}
	var proc *downloadProcess

	// Callback to detect waiting state and completion markers from subprocess output
	outputCallback := func(line string) {
		if logging.IsSubprocessWaitLine(line) {
			isWaiting.Store(true)
		} else if strings.Contains(line, "frame=") || logging.IsSubprocessProgressLine(line) {
			// If we see active download progress, we are no longer waiting.
			isWaiting.Store(false)
			hadDownloadActivity.Store(true)
			if proc != nil && proc.downloadWaitCount != nil {
				proc.downloadWaitCount.Store(0)
			}
		}

		if proc != nil &&
			proc.downloadWaitCount != nil &&
			proc.downloadWaitTriggered != nil &&
			proc.hadDownloadActivity != nil &&
			!proc.hadDownloadActivity.Load() &&
			downloadWaitRetries > 0 &&
			logging.IsSubprocessWaitLine(line) {
			count := proc.downloadWaitCount.Add(1)
			if int(count) >= downloadWaitRetries && proc.downloadWaitTriggered.CompareAndSwap(false, true) {
				proc.logger.LogEvent("WAIT", fmt.Sprintf(
					"%s is still waiting on %s%s%s after %d wait lines. Stopping it.",
					proc.downloaderName,
					ansi.ColorOrange,
					ch.Name,
					ansi.ColorReset,
					count,
				))
				if proc.cmd != nil && proc.cmd.Process != nil {
					if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
						proc.cmd.Process.Kill()
					}
				}
			}
		}

		// Track successful merge markers from downloader post-processing.
		if strings.Contains(line, "[Merger]") ||
			strings.Contains(line, "Merging formats") ||
			strings.Contains(line, "Successfully merged files into:") {
			mergerDetected.Store(true)
		}

		// Detect yt-dlp/ffmpeg postprocessing failures
		if strings.Contains(line, "ERROR: Postprocessing:") || strings.Contains(line, "Postprocessing: Conversion failed") || strings.Contains(line, "Conversion failed") {
			postprocessFailed.Store(true)
		}

		// Detect fragment/network exhaustion or repeated fragment 4xx/5xx
		if strings.Contains(line, "Did not get any data blocks") || strings.Contains(line, "fragment not found") || strings.Contains(line, "Got error: HTTP Error") || strings.Contains(line, "fragment") && strings.Contains(line, "Not Found") {
			fragmentFailure.Store(true)
		}

		// Extractor / extraction failures
		if strings.Contains(line, "ERROR: Unable to download webpage") || strings.Contains(line, "ERROR: unable to extract") || strings.Contains(line, "ERROR: No video formats") || strings.Contains(line, "ERROR: unable to download video data") {
			extractorFailed.Store(true)
		}

		// Authentication / permission failures
		if strings.Contains(line, "This video is private") || strings.Contains(line, "401 Unauthorized") || strings.Contains(line, "needs login") || strings.Contains(line, "requires authentication") {
			authFailure.Store(true)
		}

		// Disk / environment errors
		if strings.Contains(line, "Permission denied") || strings.Contains(line, "No space left on device") || strings.Contains(line, "file access error") {
			diskFailure.Store(true)
		}

		// Process termination/crash indicators
		if strings.Contains(line, "Killed") || strings.Contains(line, "segfault") || strings.Contains(line, "Traceback (most recent call last):") {
			processCrashed.Store(true)
		}

		// Track completion markers commonly emitted by twitch-dlp, livestream_dl, and ffmpeg.
		if strings.Contains(line, "[stats] Fragments") ||
			(strings.Contains(line, "frame=") && strings.Contains(line, "Lsize=")) ||
			(strings.Contains(line, "[out#") && strings.Contains(line, "muxing overhead:")) ||
			strings.Contains(line, "Successfully merged files into:") ||
			strings.Contains(line, "Finished moving files from temporary directory to output destination") {
			downloadCompleted.Store(true)
		}
	}

	// Create lockfile
	if err := lockfile.CreateLock(lockPath); err != nil {
		b.logger.LogErrorf("Error creating lockfile for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err)
		return false
	}
	b.logger.LogEvent("LOCK", fmt.Sprintf("Created: %s", lockPath))

	// Build command using the controller, with a one-shot YouTube retry override
	// when a completed download is found to still be live on the next poll.
	var cmd *exec.Cmd
	forcedDownloaderName := ""
	retryMode := ""
	if retryDownloader, ok := b.takeYTRetryDownloader(ch.ID, status.VideoID); ok {
		if controller, canRetry := b.controller.(RetryDownloaderController); canRetry {
			retryCmd, downloaderName, enabled := controller.BuildRetryDownloaderCmd(ch, status, retryDownloader)
			if enabled && retryCmd != nil {
				cmd = retryCmd
				forcedDownloaderName = downloaderName
				retryMode = retryDownloader.mode
			}
		}
	}
	if cmd == nil {
		cmd = b.controller.BuildDownloaderCmd(ch, status)
	}
	downloaderName := downloaderNameFromCommand(cmd.Path, cmd.Args)
	if forcedDownloaderName != "" {
		downloaderName = canonicalDownloaderName(forcedDownloaderName)
	}

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
		logger.LogEvent("DIAGNOSTIC", "Raw subprocess output will be shown (dlp_verbose_debug=true)")
	}

	// Force colors in subprocess output (yt-dlp, twitch-dlp)
	// Set environment variables to enable color output even when piping
	// Doesn't work, twitch-dlp already does this and yt-dlp doesn't show color with this
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1", "TERM=xterm-256color")

	if globalCfg.SaveDownloadLogs || dlpDebug {
		stdoutPipe, errOut := cmd.StdoutPipe()
		stderrPipe, errErr := cmd.StderrPipe()

		if errOut == nil && stdoutPipe != nil {
			go logging.ReadPipeAndLog(stdoutPipe, logger, downloaderName, outputCallback)
		}
		if errErr == nil && stderrPipe != nil {
			go logging.ReadPipeAndLog(stderrPipe, logger, downloaderName, outputCallback)
		}
	}

	// Log the command if dlp debug is enabled (for terminal display)
	if dlpDebug {
		logger.LogSubprocessOutput("COMMAND: "+commandStr, downloaderName)
	}

	proc = &downloadProcess{
		cmd:                   cmd,
		videoID:               status.VideoID,
		downloaderName:        downloaderName,
		lockPath:              lockPath,
		logger:                logger,
		isWaiting:             isWaiting,
		mergerDetected:        mergerDetected,
		postprocessFailed:     postprocessFailed,
		fragmentFailure:       fragmentFailure,
		extractorFailed:       extractorFailed,
		authFailure:           authFailure,
		diskFailure:           diskFailure,
		processCrashed:        processCrashed,
		downloadCompleted:     downloadCompleted,
		downloadWaitCount:     &atomic.Int32{},
		downloadWaitTriggered: &atomic.Bool{},
		hadDownloadActivity:   hadDownloadActivity,
		status:                status,
		outputCallback:        outputCallback,
		retryMode:             retryMode,
	}

	// Start command
	if err := cmd.Start(); err != nil {
		logger.LogError(fmt.Sprintf("Error starting download for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
		if b.startFallbackDownload(ch, proc) {
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

	logger.LogEventf("DOWNLOAD", "%sStarted%s %s for %s%s%s: %s", ansi.ColorGreen, ansi.ColorReset, downloaderName, ansi.ColorOrange, ch.Name, ansi.ColorReset, status.Title)

	// Store process info
	proc.startedAt = startedAt
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
	return true
}
