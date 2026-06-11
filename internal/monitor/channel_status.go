package monitor

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

// checkChannel is the core logic for checking a single channel's status.
func (b *BaseMonitor) checkChannel(ctx context.Context, cancel context.CancelFunc, ch config.Channel, wg *sync.WaitGroup, pollID uint64) error {
	defer wg.Done()

	logPrefix := b.controller.GetLogPrefix()
	debugType := logging.DebugYouTube
	if logPrefix == logPrefixTwitch {
		debugType = logging.DebugTwitch
	}

	if ctx.Err() != nil {
		return &NetworkError{Err: ctx.Err()}
	}

	newStatus, err := b.controller.CheckChannelStatus(ctx, ch, b.httpClient)
	if err != nil {
		if ctx.Err() != nil {
			return &NetworkError{Err: err}
		}

		if isConnectivityError(err) {
			// Trigger immediate connection check via the global connection monitor
			connMonitor := GetGlobalConnectionMonitor(b.controller.GetGlobalConfig())
			connMonitor.TriggerImmediateCheck()
			cancel()

			if connMonitor.IsConnected() {
				b.logger.Warn(fmt.Sprintf("Network issue while checking %s: %v", ch.Name, err))
			}

			// Wrap in NetworkError so it won't count toward backoff timers
			return &NetworkError{Err: err}
		}

		b.logger.LogErrorf("Error checking %s: %v", ch.Name, err)
		return err
	}

	b.resolvePendingYTSuccess(ch, newStatus, pollID)
	if logPrefix == logPrefixTwitch {
		b.resolvePendingTwitchSuccess(ch, pollID)
	}

	// --- SAFETY NET LOGIC (pre-lock check) ---
	if !newStatus.IsLive {
		b.statusMutex.RLock()
		previousStatus, wasTracked := b.liveStatus[ch.ID]
		b.statusMutex.RUnlock()

		b.downloadMutex.Lock()
		proc, isDownloading := b.activeDownloads[ch.ID]
		b.downloadMutex.Unlock()

		if logPrefix == logPrefixTwitch &&
			isDownloading &&
			proc.downloaderName == "twitch-dlp" &&
			proc.downloadCompleted != nil &&
			proc.downloadCompleted.Load() &&
			proc.isWaiting != nil &&
			proc.isWaiting.Load() {
			b.logger.Debug(debugType, fmt.Sprintf("API reports %s%s%s as offline and twitch-dlp is waiting after completion. Terminating downloader.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
			proc.forcedTermination.Store(true)
			if proc.cmd != nil && proc.cmd.Process != nil {
				if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
					proc.cmd.Process.Kill()
				}
			}
			// Fall through to update status to offline; waitForDownload will handle cleanup.
		} else if wasTracked && previousStatus.IsLive && isDownloading && proc.videoID == newStatus.LastBroadcastID {
			// Check if the downloader is in a waiting state (e.g. twitch-dlp retrying after stream end)
			if proc.isWaiting != nil && proc.isWaiting.Load() {
				b.logger.Debug(debugType, fmt.Sprintf("API reports %s%s%s as offline and downloader is waiting. Terminating downloader.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
				proc.forcedTermination.Store(true)
				if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
					proc.cmd.Process.Kill()
				}
				// Fall through to update status to offline; waitForDownload will handle cleanup
			} else {
				b.logger.Debug(debugType, fmt.Sprintf("API reports %s%s%s as offline, but download is active for same stream ID (%s). Ignoring.", ansi.ColorOrange, ch.Name, ansi.ColorReset, proc.videoID))
				return nil // Ignore this offline signal.
			}
		}
	}
	// --- END SAFETY NET ---

	b.statusMutex.Lock()
	defer b.statusMutex.Unlock()

	previousStatus, wasTracked := b.liveStatus[ch.ID]

	// Handle state changes
	if newStatus.IsLive {
		// Filter check
		matchesFilter := false
		if len(ch.Filters) == 0 { // If no filters, always match
			matchesFilter = true
		} else {
			for _, filter := range ch.Filters {
				if matched, _ := regexp.MatchString(filter, newStatus.Title); matched {
					matchesFilter = true
					break
				}
			}
		}

		if !matchesFilter {
			if wasTracked && previousStatus.IsLive {
				b.logger.LogEventf("SKIP", "%s%s%s is live but filtered out: %s", ansi.ColorOrange, ch.Name, ansi.ColorReset, newStatus.Title)
				b.liveStatus[ch.ID] = models.LiveInfo{IsLive: false}
			}
			return nil
		}

		if !wasTracked || !previousStatus.IsLive {
			b.logger.LogEventf("LIVE", "%s%s%s is now %sLIVE%s: %s", ansi.ColorOrange, ch.Name, ansi.ColorReset, ansi.ColorGreen, ansi.ColorReset, newStatus.Title)
		}
		b.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline (genuine case, safety net already passed)
		if wasTracked && previousStatus.IsLive {
			b.logger.LogEventf("OFFLINE", "%s%s%s %shas gone offline%s.", ansi.ColorOrange, ch.Name, ansi.ColorReset, ansi.ColorRed, ansi.ColorReset)
		}
		b.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
	return nil
}
