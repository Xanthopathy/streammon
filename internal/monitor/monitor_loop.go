package monitor

import (
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

	// Configure the main check ticker
	if err != nil {
		b.logger.LogErrorf("Invalid poll_interval, defaulting to 60s. Error: %v", err)
		pollInterval = 60 * time.Second
	}

	consecutiveErrors := 0

	for {
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
