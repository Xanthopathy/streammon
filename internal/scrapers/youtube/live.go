package youtube

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"streammon/internal/models"
	"streammon/internal/util"
)

// CheckYouTubeViaLivePage performs a check by navigating to the channel's /live endpoint.
// YouTube redirects this URL to the active livestream if one exists.
// We inspect the resulting page for "isLive":true markers to distinguish actual streams from VOD redirects.
func CheckYouTubeViaLivePage(httpClient *http.Client, channelID string, channelName string, logger *util.Logger) (models.LiveInfo, error) {
	liveURL := fmt.Sprintf("https://www.youtube.com/channel/%s/live", channelID)
	logger.Debug("YouTubeAPI", fmt.Sprintf("Checking /live endpoint for %s%s%s (%s): %s", util.ColorOrange, channelName, util.ColorReset, channelID, liveURL))

	req, err := http.NewRequest("GET", liveURL, nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create /live request for %s%s%s: %v", util.ColorOrange, channelName, util.ColorReset, err)
		logger.Debug("YouTubeAPI", errorMsg)
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
		errorMsg := fmt.Sprintf("Failed to fetch /live for %s%s%s: %v", util.ColorOrange, channelName, util.ColorReset, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorMsg := fmt.Sprintf("/live request for %s%s%s returned non-200 status: %s", util.ColorOrange, channelName, util.ColorReset, resp.Status)
		logger.Debug("YouTubeAPI", errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Read body (limit to 1MB to capture the scripts containing ytInitialPlayerResponse)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read /live body for %s%s%s: %v", util.ColorOrange, channelName, util.ColorReset, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return models.LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	body := string(bodyBytes)

	// Check for strict live indicators.
	// If the channel is offline, /live often redirects to the last VOD, which looks like a video page but has isLive:false (or missing).
	isStatusLive := strings.Contains(body, `"status":"LIVE"`)

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
		logger.Debug("YouTubeAPI", fmt.Sprintf("/live redirect for %s%s%s is a scheduled event. Starts: %s (in %v)", util.ColorOrange, channelName, util.ColorReset, scheduledTime, timeUntil))

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
		logger.Debug("YouTubeAPI", fmt.Sprintf("%s%s%s /live page did not contain live indicators (likely redirect to VOD or channel home).", util.ColorOrange, channelName, util.ColorReset))
		return models.LiveInfo{IsLive: false}, nil
	}

	// Extract Video ID
	videoIDRegex := regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
	videoIDMatch := videoIDRegex.FindStringSubmatch(body)
	if len(videoIDMatch) < 2 {
		logger.Debug("YouTubeAPI", fmt.Sprintf("Detected live status for %s%s%s but could not extract Video ID.", util.ColorOrange, channelName, util.ColorReset))
		return models.LiveInfo{IsLive: false}, nil
	}
	videoID := videoIDMatch[1]

	// Extract Title (fallback to HTML title tag if JSON parsing is too complex for regex)
	title := "Unknown Title"
	// HTML title usually follows: <title>Stream Title - YouTube</title>
	titleRegex := regexp.MustCompile(`<title>(.*?) - YouTube</title>`)
	titleMatch := titleRegex.FindStringSubmatch(body)
	if len(titleMatch) >= 2 {
		title = titleMatch[1]
	}

	logger.Debug("YouTubeAPI", fmt.Sprintf("Found live stream via /live: %s (%s)", title, videoID))

	return models.LiveInfo{
		IsLive:    true,
		VideoID:   videoID,
		Title:     title,
		CreatedAt: time.Now(), // Approximate since we don't parse the exact start time
	}, nil
}
