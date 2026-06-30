package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"streammon/internal/models"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

// CheckYouTubeViaLivePage performs a check by navigating to the channel's /live endpoint.
// YouTube redirects this URL to the active livestream if one exists.
// We inspect the resulting page for "isLive":true markers to distinguish actual streams from VOD redirects.
func CheckYouTubeViaLivePage(ctx context.Context, httpClient *http.Client, channelID string, channelName string, logger *logging.Logger) (models.LiveInfo, error) {
	liveURL := fmt.Sprintf("https://www.youtube.com/channel/%s/live", channelID)
	logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Checking /live endpoint for %s%s%s (%s): %s", ansi.ColorOrange, channelName, ansi.ColorReset, channelID, liveURL))

	req, err := http.NewRequestWithContext(ctx, "GET", liveURL, nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create /live request for %s%s%s: %v", ansi.ColorOrange, channelName, ansi.ColorReset, err)
		logger.Debug(logging.DebugYouTubeAPI, errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Mimic a real browser navigation to ensure we get the proper player response
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := httpClient.Do(req)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to fetch /live for %s%s%s: %v", ansi.ColorOrange, channelName, ansi.ColorReset, err)
		logger.Debug(logging.DebugYouTubeAPI, errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorMsg := fmt.Sprintf("/live request for %s%s%s returned non-200 status: %s", ansi.ColorOrange, channelName, ansi.ColorReset, resp.Status)
		logger.Debug(logging.DebugYouTubeAPI, errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Read body (limit to 1MB to capture the scripts containing ytInitialPlayerResponse)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read /live body for %s%s%s: %v", ansi.ColorOrange, channelName, ansi.ColorReset, err)
		logger.Debug(logging.DebugYouTubeAPI, errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	body := string(bodyBytes)

	// Extract Video ID from canonical URL - much safer than finding the first "videoId" match anywhere
	canonicalRegex := regexp.MustCompile(`<link rel="canonical" href="https://www.youtube.com/watch\?v=([a-zA-Z0-9_-]{11})">`)
	canonicalMatch := canonicalRegex.FindStringSubmatch(body)

	var videoID string
	if len(canonicalMatch) >= 2 {
		videoID = canonicalMatch[1]
	}

	// Verify channel ID parity to ensure we aren't catching sidebar/featured channels
	// Look for the owner of the main content (watch page or channel page)
	chanIDRegex := regexp.MustCompile(`"(?:channelId|externalId|ownerDocId)":"(UC[a-zA-Z0-9_-]{22})"`)
	chanIDMatches := chanIDRegex.FindAllStringSubmatch(body, -1)
	idMatched := false
	for _, match := range chanIDMatches {
		if match[1] == channelID {
			idMatched = true
			break
		}
	}

	if !idMatched {
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("%s%s%s /live page did not contain the expected channel ID %s. Likely redirect to suggestion or featured channel.", ansi.ColorOrange, channelName, ansi.ColorReset, channelID))
		return models.LiveInfo{IsLive: false}, nil
	}

	// Check for strict live indicators.
	// If the channel is offline, /live often redirects to the last VOD, which looks like a video page but has isLive:false (or missing).
	// We anchor the check to the video ID to avoid false positives from suggested live videos.
	var isStatusLive bool
	if videoID != "" {
		// Look for "status":"LIVE" or "isLive":true in proximity to the video ID (within ~1KB)
		// This ensures the live status belongs to the main video.
		idPos := strings.Index(body, fmt.Sprintf(`"videoId":"%s"`, videoID))
		if idPos != -1 {
			searchRange := body[idPos : min(idPos+1024, len(body))]
			isStatusLive = strings.Contains(searchRange, `"status":"LIVE"`) || strings.Contains(searchRange, `"isLive":true`)
		}
	}

	// Double check: if we didn't find videoID from canonical, fall back to global check but still be cautious
	if !isStatusLive {
		isStatusLive = strings.Contains(body, `"status":"LIVE"`)
	}

	// Check for scheduled start time (detect upcoming streams/premieres)
	// "scheduledStartTime":"1678900000"
	scheduledTimeRegex := regexp.MustCompile(`"scheduledStartTime":"(\d+)"`)
	scheduledMatch := scheduledTimeRegex.FindStringSubmatch(body)
	var isScheduled bool
	var scheduledTime time.Time

	if len(scheduledMatch) >= 2 {
		ts, err := strconv.ParseInt(scheduledMatch[1], 10, 64)
		if err == nil {
			scheduledTime = time.Unix(ts, 0)
			isScheduled = true
		}
	}

	if isScheduled {
		timeUntil := time.Until(scheduledTime)
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("/live redirect for %s%s%s is a scheduled event. Starts: %s (in %v)", ansi.ColorOrange, channelName, ansi.ColorReset, scheduledTime, timeUntil))

		// If it's scheduled but not explicitly LIVE yet, treat it as offline.
		// This filters out "glorified chatrooms" (streams scheduled far in the future).
		if !isStatusLive {
			return models.LiveInfo{IsLive: false}, nil
		}
	}

	// Fallback: If not scheduled, check for generic isLive flag.
	// We only use this loose check if we didn't find specific scheduling info to avoid false positives on upcoming events.
	isLiveLoose := strings.Contains(body, `"isLive":true`)

	if !isStatusLive && !isLiveLoose {
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("%s%s%s /live page did not contain live indicators (likely redirect to VOD or channel home).", ansi.ColorOrange, channelName, ansi.ColorReset))
		return models.LiveInfo{IsLive: false}, nil
	}

	// Extract Video ID (if not already found via canonical)
	if videoID == "" {
		videoIDRegex := regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
		videoIDMatch := videoIDRegex.FindStringSubmatch(body)
		if len(videoIDMatch) < 2 {
			logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Detected live status for %s%s%s but could not extract Video ID.", ansi.ColorOrange, channelName, ansi.ColorReset))
			return models.LiveInfo{IsLive: false}, nil
		}
		videoID = videoIDMatch[1]
	}

	// Extract Title (fallback to HTML title tag if JSON parsing is too complex for regex)
	title := "Unknown Title"
	// HTML title usually follows: <title>Stream Title - YouTube</title>
	titleRegex := regexp.MustCompile(`<title>(.*?) - YouTube</title>`)
	titleMatch := titleRegex.FindStringSubmatch(body)
	if len(titleMatch) >= 2 {
		title = titleMatch[1]
	}

	logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Found live stream via /live: %s (%s)", title, videoID))

	return models.LiveInfo{
		IsLive:    true,
		VideoID:   videoID,
		Title:     title,
		CreatedAt: time.Now(), // Approximate since we don't parse the exact start time
	}, nil
}
