package monitor

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"streammon/internal/config"
)

// checkAllChannels concurrently checks all configured channels with request pacing.
// Uses bounded concurrency (4 requests) plus request spacing to avoid API rate limits.
// Balances freshness target (poll_interval) with safety limit (max_requests_per_second).
func (b *BaseMonitor) checkAllChannels() int {
	pollID := b.pollGeneration.Add(1)
	channels := b.controller.GetChannels()
	connMonitor := GetGlobalConnectionMonitor(b.controller.GetGlobalConfig())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !connMonitor.IsConnected() {
					cancel()
					return
				}
			}
		}
	}()

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
			if !b.rpsWarningSent {
				b.logger.Warn(fmt.Sprintf(
					"Configured poll_interval (%s) is too short for %d channels with max_requests_per_second limit. "+
						"Forcing request spacing to %s to respect safety limits. "+
						"Effective poll cycle will be slower than configured.",
					pollInterval, channelCount, rpsSpacing))
				b.rpsWarningSent = true
			}
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
		if ctx.Err() != nil || !connMonitor.IsConnected() {
			cancel()
			break
		}

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
			if !sleepWithContext(ctx, actualSleep) {
				break
			}
		}

		// Acquire semaphore slot (blocking if full)
		acquiredSlot := false
		select {
		case sem <- struct{}{}:
			acquiredSlot = true
		case <-ctx.Done():
		}

		if !acquiredSlot {
			break
		}

		wg.Add(1)
		go func(c config.Channel) {
			// Ensure we release the slot when this goroutine finishes
			defer func() { <-sem }()
			if err := b.checkChannel(ctx, cancel, c, &wg, pollID); err != nil {
				// Only count non-network errors toward backoff timers.
				// NetworkErrors indicate connection issues, which are handled by the
				// global connection monitor (it will pause operations anyway).
				if !IsNetworkError(err) {
					errorCount.Add(1)
				}
			}
		}(ch)
	}
	wg.Wait()
	return int(errorCount.Load())
}
