package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/scrapers/youtube"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

func (m *YTMonitor) checkMembersIfEnabled(ctx context.Context, ch config.Channel) (models.LiveInfo, error) {
	if !m.shouldCheckMembers(ch) {
		return models.LiveInfo{IsLive: false}, nil
	}

	memberInfo, memberErr := youtube.CheckYouTubeViaMembersPlaylist(
		ctx,
		m.cookiesFileAbs(),
		m.cfg.Scraper.MemberCheckArgs,
		ch.ID,
		ch.Name,
		m.base.logger,
	)
	if memberErr != nil {
		m.base.logger.Debug(
			logging.DebugYouTubeAPI,
			fmt.Sprintf(
				"Member check failed for %s%s%s: %v",
				ansi.ColorOrange,
				ch.Name,
				ansi.ColorReset,
				memberErr,
			),
		)
		return models.LiveInfo{IsLive: false}, memberErr
	}

	return memberInfo, nil
}

// CheckChannelStatus for YouTube uses RSS parsing to confirm a stream is live.
// Only launches yt-dlp if both checks confirm the stream exists.
func (m *YTMonitor) CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	// Parse the ignore_older_than duration from config
	ignoreOlderThan, err := time.ParseDuration(m.cfg.Scraper.IgnoreOlderThan)
	if err != nil {
		// Default to 24 hours if parse fails
		ignoreOlderThan = 24 * time.Hour
		m.base.logger.Debug(logging.DebugYouTube, fmt.Sprintf("Failed to parse ignore_older_than for %s%s%s: %v, using default 24h", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
	}

	fallbackDuration, err := time.ParseDuration(m.cfg.Scraper.FallbackDuration)
	if err != nil {
		fallbackDuration = 15 * time.Minute
		m.base.logger.Debug(logging.DebugYouTube, fmt.Sprintf("Failed to parse fallback_duration, using default 15m: %v", err))
	}

	// Determine check order based on config
	// Might add Invidious/Holodex later
	var methods []string
	defaultMethod := m.cfg.Scraper.CheckMethod

	// Check if channel is in fallback mode
	currentState, hasState := m.getFallbackState(ch.ID)
	now := time.Now()
	if hasState && now.Before(currentState.expiresAt) {
		// Use the fallback method as primary
		// We assume only 2 methods for now. If persistent method is live, try live then rss.
		if currentState.method == "live" {
			methods = []string{"live", "rss"}
		} else {
			methods = []string{"rss", "live"}
		}
	} else if defaultMethod == "live" {
		methods = []string{"live", "rss"}
	} else {
		// Default to RSS first
		methods = []string{"rss", "live"}
	}

	var lastErr error

	for i, method := range methods {
		if ctx.Err() != nil {
			return models.LiveInfo{}, ctx.Err()
		}

		var info models.LiveInfo
		var err error

		switch method {
		case "rss":
			info, err = youtube.CheckYouTubeViaRSS(ctx, httpClient, ch.ID, ch.Name, m.base.logger, ignoreOlderThan)
		case "live":
			info, err = youtube.CheckYouTubeViaLivePage(ctx, httpClient, ch.ID, ch.Name, m.base.logger)
		default:
			continue
		}

		if err == nil {
			// If we succeeded using a method that isn't the configured default, set/refresh fallback state
			// This makes the fallback "sticky" for a duration (fallbackDuration)
			if method != defaultMethod {
				m.setFallbackState(ch.ID, fallbackState{
					method:    method,
					expiresAt: now.Add(fallbackDuration),
				})
			} else {
				// We succeeded with the default method. Clear any fallback state to revert to normal behavior.
				m.clearFallbackState(ch.ID)
			}

			if !info.IsLive {
				memberInfo, memberErr := m.checkMembersIfEnabled(ctx, ch)
				if memberErr == nil && memberInfo.IsLive {
					return memberInfo, nil
				}
			}

			return info, nil
		}

		if isConnectivityError(err) {
			return models.LiveInfo{}, err
		}

		// Identify fallback method if one exists
		var fallbackName string
		if i+1 < len(methods) {
			fallbackName = methods[i+1]
			m.stats.Add(ch.Name, fallbackName)
		}

		// Log failure only if API verbose is on (to reduce spam)
		m.base.logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Method '%s' failed for %s%s%s: %v. Trying fallback to '%s'...", method, ansi.ColorOrange, ch.Name, ansi.ColorReset, err, fallbackName))
		lastErr = err
	}

	memberInfo, memberErr := m.checkMembersIfEnabled(ctx, ch)
	if memberErr == nil && memberInfo.IsLive {
		return memberInfo, nil
	}

	return models.LiveInfo{}, fmt.Errorf("all check methods failed (last error: %w)", lastErr)
}
