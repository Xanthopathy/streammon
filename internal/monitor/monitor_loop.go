package monitor

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"streammon/internal/util"
)

// Run starts the main monitoring loop.
func (b *BaseMonitor) Run() {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	channels := b.controller.GetChannels()
	logPrefix := b.controller.GetLogPrefix()

	// Seed random for jitter
	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Initialize the global download semaphore using the value from the global config.
	initializeDownloadSlots(globalCfg.MaxConcurrentDownloads)

	b.logger.Logf("Monitor started for %d channels.", len(channels))
	b.logger.Logf("Working Directory: %s", streamMonCfg.WorkingDirectory)

	// Log request spacing configuration
	channelCount := len(channels)
	pollInterval, err := b.controller.GetPollInterval()
	if err == nil && channelCount > 0 {
		maxRPS := b.controller.GetMaxRequestsPerSecond()
		if maxRPS <= 0 {
			maxRPS = 2
		}

		idealSpacing := pollInterval / time.Duration(channelCount)
		rpsSpacing := time.Second / time.Duration(int(maxRPS))
		effectiveSpacing := idealSpacing
		if rpsSpacing > idealSpacing {
			effectiveSpacing = rpsSpacing
		}

		b.logger.Logf("Configured poll_interval: %v | Channels: %d | Effective request spacing: ~%v", pollInterval, channelCount, effectiveSpacing)
	}

	// Create working directory if it doesn't exist
	if _, err := os.Stat(streamMonCfg.WorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(streamMonCfg.WorkingDirectory, 0755)
		if err != nil {
			b.logger.LogErrorf("Error creating working directory: %v", err)
			return
		}
		b.logger.Logf("Created working directory: %s", streamMonCfg.WorkingDirectory)
	}

	// Load archive.txt if enabled to prevent re-downloads
	shouldArchive := false
	if logPrefix == "YT" && globalCfg.YoutubeArchiveDownloads {
		shouldArchive = true
	} else if logPrefix == "Twitch" && globalCfg.TwitchArchiveDownloads {
		shouldArchive = true
	}

	if shouldArchive {
		archivePath := filepath.Join(streamMonCfg.WorkingDirectory, "archive.txt")
		if lines, err := util.ReadLinesToSet(archivePath); err == nil {
			b.archivedVideos = lines
			b.logger.Logf("Loaded %d archived video IDs.", len(b.archivedVideos))
		}
	}

	// Start the download manager in the background
	go b.manageDownloads()

	// Start connection monitor in background
	go b.monitorConnection()

	// Configure the main check ticker
	if err != nil {
		b.logger.LogErrorf("Invalid poll_interval, defaulting to 60s. Error: %v", err)
		pollInterval = 60 * time.Second
	}

	consecutiveErrors := 0

	for {
		// Check connection status before doing any work
		b.pauseCond.L.Lock()
		for !b.isConnected {
			// Wait releases the lock and suspends the goroutine until Broadcast/Signal is called
			b.pauseCond.Wait()
		}
		b.pauseCond.L.Unlock()

		// Run check and track errors
		errorCount := b.checkAllChannels()

		// Switch to fixed-delay scheduling aka sleep for the full interval AFTER the work is done.
		// Previously we subtracted work duration, which dangerously reduced quiet time as the channel list grew.
		sleepDuration := pollInterval

		// Add random jitter (-10% to +10%) to the poll interval to mitigate bot pattern recognition
		// Example: 60s becomes something between 54s and 66s.
		jitterPercent := 0.10
		jitterRange := int64(float64(pollInterval) * jitterPercent)
		sleepDuration += time.Duration(rand.Int63n(jitterRange*2) - jitterRange)

		// Backoff logic if errors occurred
		if errorCount > 0 {
			consecutiveErrors++
			// Add 3 minute per consecutive error run, cap at 15 minutes
			backoff := time.Duration(consecutiveErrors) * 3 * time.Minute
			if backoff > 15*time.Minute {
				backoff = 15 * time.Minute
			}
			b.logger.Logf("Detected %d errors during poll. Staggering next poll by +%v (Consecutive failures: %d)", errorCount, backoff, consecutiveErrors)
			sleepDuration += backoff
		} else {
			consecutiveErrors = 0
		}

		time.Sleep(sleepDuration)
	}
}

// monitorConnection runs in the background and periodically checks internet connectivity.
// If connection is lost, it sets isConnected=false (which blocks the main loop).
// When connection is restored, it sets isConnected=true and wakes up the main loop.
func (b *BaseMonitor) monitorConnection() {
	normalInterval := 10 * time.Second
	recoveryInterval := 5 * time.Second

	timer := time.NewTimer(normalInterval)
	defer timer.Stop()

	// Hysteresis counters to prevent flapping
	consecutiveSuccess := 0
	consecutiveFailure := 0
	const threshold = 3

	sysLogger := util.NewLogger(b.controller.GetGlobalConfig(), "System", util.ColorCyan)

	for {
		// Wait for timer or trigger
		select {
		case <-b.connCheckTrigger:
			// Immediate check requested by checker.go.
			// Drain the timer so we don't double-check immediately after.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			// Periodic check
		}

		connected := util.CheckInternetConnection()

		b.pauseCond.L.Lock()
		if b.isConnected {
			if connected {
				consecutiveFailure = 0 // Reset failure count
			} else {
				consecutiveFailure++
				// Log a warning on the first failure so the user knows why checks might be failing
				if consecutiveFailure == 1 {
					sysLogger.Warn("Connection check failed. Verifying stability...")
				}
				if consecutiveFailure >= threshold {
					sysLogger.Logf("%sConnection lost (confirmed).%s Pausing monitors...", util.ColorRed, util.ColorReset)
					b.isConnected = false
					consecutiveSuccess = 0 // Reset success count for recovery
				}
			}
		} else {
			// Currently disconnected
			if connected {
				consecutiveSuccess++
				sysLogger.Debug("System", fmt.Sprintf("Connection check passed (%d/%d)...", consecutiveSuccess, threshold))
				if consecutiveSuccess >= threshold {
					sysLogger.Logf("%sConnection restored (stable).%s Resuming operations...", util.ColorGreen, util.ColorReset)
					b.isConnected = true
					consecutiveFailure = 0
					// Wake up all goroutines waiting on this condition (e.g. main loop, manager)
					b.pauseCond.Broadcast()
				}
			} else {
				consecutiveSuccess = 0 // Connection still flaky, reset success count
			}
		}

		currentState := b.isConnected
		b.pauseCond.L.Unlock()

		if currentState {
			timer.Reset(normalInterval)
		} else {
			// Check more frequently when offline to resume quickly
			timer.Reset(recoveryInterval)
		}
	}
}
