package monitor

import (
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// checkAllChannels concurrently checks all configured channels with request pacing.
// Uses bounded concurrency (4 requests) plus request spacing to avoid API rate limits.
// Balances freshness target (poll_interval) with safety limit (max_requests_per_second).
func (b *BaseMonitor) checkAllChannels() int {
	channels := b.controller.GetChannels()

	var wg sync.WaitGroup
	var errorCount atomic.Int32

	// Bounded concurrency prevents burst patterns that trigger detection.
	concurrencyLimit := 4
	sem := make(chan struct{}, concurrencyLimit)

	// Calculate request spacing from two constraints:
	// 1. Freshness target: spread requests across poll_interval
	// 2. Safety limit: enforce max_requests_per_second
	// Use the more conservative (larger) spacing from these two.
	pollInterval := time.Duration(0)
	if interval, err := b.controller.GetPollInterval(); err == nil {
		pollInterval = interval
	} else {
		pollInterval = 60 * time.Second
	}

	channelCount := len(channels)
	var requestSpacing time.Duration
	if channelCount > 0 {
		// Ideal spacing from poll interval
		idealSpacing := pollInterval / time.Duration(channelCount)

		// Minimum spacing from max_requests_per_second
		// Use float64 math to avoid precision loss with fractional RPS (e.g., 1.5 RPS)
		maxRPS := b.controller.GetMaxRequestsPerSecond()
		if maxRPS <= 0 {
			maxRPS = 2 // Default: 2 requests per second
		}
		rpsSpacing := time.Duration(float64(time.Second) / maxRPS)

		// Use whichever spacing is more conservative (larger)
		requestSpacing = idealSpacing
		if rpsSpacing > idealSpacing {
			requestSpacing = rpsSpacing
		}
	}

	// Stagger requests with jittered delays to avoid bot-like perfect timing.
	// Random jitter (±25% of requestSpacing) simulates organic traffic patterns
	// and prevents detection of mathematically perfect request rhythms.
	// We use time.Sleep() instead of time.Ticker to avoid:
	// 1. Buffered tick bursts when goroutines are slow (ticker buffers 1 tick)
	// 2. Perfect rhythmic patterns that bot detectors flag
	jitterPercent := 0.25

	for _, ch := range channels {
		wg.Add(1)

		// Apply randomized jitter to spacing if configured
		if requestSpacing > 0 {
			jitterRange := int64(float64(requestSpacing) * jitterPercent)
			// Ensure we don't divide by zero if jitterRange is 0
			var randomJitter time.Duration
			if jitterRange > 0 {
				randomJitter = time.Duration(rand.Int63n(jitterRange*2)) - time.Duration(jitterRange)
			}
			actualSleep := requestSpacing + randomJitter
			if actualSleep < 100*time.Millisecond {
				actualSleep = 100 * time.Millisecond // Minimum safety floor
			}
			time.Sleep(actualSleep)
		}

		// Acquire semaphore slot (blocking if full)
		sem <- struct{}{}

		go func(c config.Channel) {
			// Ensure we release the slot when this goroutine finishes
			defer func() { <-sem }()
			if err := b.checkChannel(c, &wg); err != nil {
				errorCount.Add(1)
			}
		}(ch)
	}
	wg.Wait()
	return int(errorCount.Load())
}

// checkChannel is the core logic for checking a single channel's status.
func (b *BaseMonitor) checkChannel(ch config.Channel, wg *sync.WaitGroup) error {
	defer wg.Done()

	globalCfg := b.controller.GetGlobalConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	newStatus, err := b.controller.CheckChannelStatus(ch, b.httpClient)
	if err != nil {
		fmt.Printf("%s [%s%s%s] Error checking %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return err
	}

	// --- SAFETY NET LOGIC (pre-lock check) ---
	if !newStatus.IsLive {
		b.statusMutex.RLock()
		previousStatus, wasTracked := b.liveStatus[ch.ID]
		b.statusMutex.RUnlock()

		b.downloadMutex.Lock()
		proc, isDownloading := b.activeDownloads[ch.ID]
		b.downloadMutex.Unlock()

		if wasTracked && previousStatus.IsLive && isDownloading && proc.videoID == newStatus.LastBroadcastID {
			// Check if the downloader is in a waiting state (e.g. twitch-dlp retrying after stream end)
			if proc.isWaiting != nil && proc.isWaiting.Load() {
				util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("API reports %s as offline and downloader is waiting. Terminating downloader.", ch.Name))
				if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
					proc.cmd.Process.Kill()
				}
				// Fall through to update status to offline; waitForDownload will handle cleanup
			} else {
				util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("API reports %s as offline, but download is active for same stream ID (%s). Ignoring.", ch.Name, proc.videoID))
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
				fmt.Printf("%s [%s%s%s] %s is live but filtered out: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, newStatus.Title)
				b.liveStatus[ch.ID] = LiveInfo{IsLive: false}
			}
			return nil
		}

		if !wasTracked || !previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s %sis now LIVE%s: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, util.ColorGreen, util.ColorReset, newStatus.Title)
		}
		b.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline (genuine case, safety net already passed)
		if wasTracked && previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s%s has gone offline%s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, util.ColorRed, ch.Name, util.ColorReset)
		}
		b.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
	return nil
}
