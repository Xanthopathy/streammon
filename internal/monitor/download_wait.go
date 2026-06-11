package monitor

import (
	"fmt"
	"os"
	"time"

	"streammon/internal/config"
	"streammon/internal/util/ansi"
	"streammon/internal/util/lockfile"
	"streammon/internal/util/terminal"
)

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
			proc.logger.LogEventf("DIAGNOSTIC", "%s exit code: %d | merger_detected: %v | file_exists: %v", proc.downloaderName, exitCode, mergerSuccess, outputFileExists)
		case "twitch-dlp":
			proc.logger.LogEventf("DIAGNOSTIC", "%s exit code: %d | completion_detected: %v | file_exists: %v", proc.downloaderName, exitCode, downloadComplete, outputFileExists)
		case "livestream_dl":
			proc.logger.LogEventf("DIAGNOSTIC", "%s exit code: %d | completion_detected: %v | merger_detected: %v | file_exists: %v", proc.downloaderName, exitCode, downloadComplete, mergerSuccess, outputFileExists)
		default:
			proc.logger.LogEventf("DIAGNOSTIC", "%s exit code: %d | completion_detected: %v | merger_detected: %v | file_exists: %v", proc.downloaderName, exitCode, downloadComplete, mergerSuccess, outputFileExists)
		}
	}

	// Determine final success status
	isSuccess := false
	if proc.forcedTermination.Load() {
		// Forced termination by monitor (stream went offline)
		proc.logger.LogEventf("DOWNLOAD", "Download for %s%s%s stopped by monitor (stream offline).", ansi.ColorOrange, ch.Name, ansi.ColorReset)
		isSuccess = true // Treat forced termination as success (meaningful data captured)
	} else if proc.downloaderName == "yt-dlp" && mergerSuccess && outputFileExists {
		// Both success conditions met
		proc.logger.LogEventf("SUCCESS", "Download for %s%s%s finished successfully.", ansi.ColorOrange, ch.Name, ansi.ColorReset)
		cleanupYTDLPResidue(proc.cmd.Dir, proc, proc.logger)
		isSuccess = true
	} else if proc.downloaderName == "twitch-dlp" && outputFileExists && (downloadComplete || exitCode == 0) {
		// twitch-dlp does not emit yt-dlp merger markers; use its own completion markers and file output.
		proc.logger.LogEventf("SUCCESS", "Download for %s%s%s finished successfully.", ansi.ColorOrange, ch.Name, ansi.ColorReset)
		isSuccess = true
	} else if proc.downloaderName == "livestream_dl" && outputFileExists && exitCode == 0 {
		proc.logger.LogEventf("SUCCESS", "Download for %s%s%s finished successfully with livestream_dl.", ansi.ColorOrange, ch.Name, ansi.ColorReset)
		cleanupYTDLPResidueForDownloader(proc.cmd.Dir, proc.videoID, proc.previousDownloader, proc.logger)
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
		if b.startFallbackDownload(ch, proc) {
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

	shouldLogSlots := (logPrefix == logPrefixTwitch && globalCfg.TwitchVerboseDebug) || (logPrefix == logPrefixYouTube && globalCfg.YoutubeVerboseDebug)
	if shouldLogSlots {
		proc.logger.LogEventf("SLOT", "Released download slot for %s%s%s. Slots used: %d/%d.", ansi.ColorOrange, ch.Name, ansi.ColorReset, len(downloadSlots), cap(downloadSlots))
	}

	// Finalize success or set pending state for YouTube
	if isSuccess {
		if logPrefix == logPrefixYouTube && proc.retryMode == ytRetryModeOfflineVOD {
			proc.logger.LogEvent("ARCHIVE", "Final VOD retry completed after stream ended. Archiving this YouTube download.")
			b.finalizeSuccessfulDownload(ch.ID, proc.videoID, proc.logger)
		} else if logPrefix == logPrefixYouTube && !proc.forcedTermination.Load() {
			b.setPendingYTSuccess(ch.ID, proc.videoID, proc.status.Source, proc.downloaderName)
			proc.logger.LogEvent("ARCHIVE", "Waiting for the next YT poll before archiving this download.")
			proc.logger.LogEventf("ARCHIVE", "Pending YouTube success recorded for %s; the next poll will either archive it or retry with another downloader if the stream is still live.", proc.videoID)
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
