package monitor

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"streammon/internal/util"
)

// --- RSS Feed Structures ---

type YouTubeRSSFeed struct {
	Entries []YouTubeRSSEntry `xml:"entry"`
}

type YouTubeRSSEntry struct {
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Published time.Time `xml:"published"`
	Updated   time.Time `xml:"updated"`
	VideoID   string    `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string    `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0"

// CheckYouTubeViaRSS parses the YouTube RSS feed for a channel to detect recent videos.
// It returns LiveInfo with the latest recent video that matches the age filter.
// Unlike strict "live" detection, this approach is simpler and more reliable:
// - Just checks if video's updated timestamp is recent (within ignore_older_than)
// - Lets yt-dlp determine if it's actually a live stream
func CheckYouTubeViaRSS(httpClient *http.Client, channelID string, channelName string, logger *util.Logger, ignoreOlderThan time.Duration) (LiveInfo, error) {
	rssURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	logger.Debug("YouTubeAPI", fmt.Sprintf("Fetching RSS feed for %s (%s): %s", channelName, channelID, rssURL))

	req, err := http.NewRequest("GET", rssURL, nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create request for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	// Standard browser headers to avoid naked request detection
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("sec-fetch-user", "?1")

	resp, err := httpClient.Do(req)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to fetch RSS feed for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorMsg := fmt.Sprintf("RSS feed request returned non-200 status: %s", resp.Status)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body) // Consider replacing with io.LimitReader(resp.Body, 1024*512) if YT sends a bomb
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read RSS feed for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	logger.Debug("YouTubeAPI", fmt.Sprintf("RSS feed for %s (%s) (first 1000 chars): %s", channelName, channelID, string(body[:min(1000, len(body))])))

	// Parse the RSS feed
	var feed YouTubeRSSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		errorMsg := fmt.Sprintf("Failed to parse RSS feed for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Look for the latest video that's recent enough
	now := time.Now()
	cutoff := now.Add(-ignoreOlderThan)

	for _, entry := range feed.Entries {
		// Prefer Updated timestamp for age check (when YouTube last updated the entry)
		// Fall back to Published if Updated is not set
		timestampToCheck := entry.Updated
		if timestampToCheck.IsZero() {
			timestampToCheck = entry.Published
		}

		// Skip very old entries
		if timestampToCheck.Before(cutoff) {
			logger.Debug("YouTubeAPI", fmt.Sprintf("Skipping %s from %s: too old (Updated=%s < cutoff=%s)", entry.VideoID, channelName, timestampToCheck, cutoff))
			continue
		}

		// Found a recent video that yt-dlp can try to download
		if entry.VideoID != "" {
			logger.Debug("YouTubeAPI", fmt.Sprintf("Found recent video from RSS: %s from %s (Published=%s, Updated=%s, Title=%s)", entry.VideoID, channelName, entry.Published, entry.Updated, entry.Title))
			return LiveInfo{
				IsLive:    true, // Mark as "live" for processing, yt-dlp will determine actual status
				VideoID:   entry.VideoID,
				Title:     entry.Title,
				CreatedAt: entry.Updated,
			}, nil
		}
	}

	logger.Debug("YouTubeAPI", fmt.Sprintf("No recent videos found in RSS feed for %s (cutoff=%s)", channelName, cutoff))
	return LiveInfo{IsLive: false}, nil
}

// CheckLiveYouTube checks if a channel has recent videos worth downloading.
// 1. Fetch the RSS feed
// 2. Check if the latest video's Updated timestamp is recent (within ignore_older_than)
// 3. Return the video details - let yt-dlp determine if it's actually a live stream
// This avoids issues with strict live-detection methods and rate limiting.
func CheckLiveYouTube(httpClient *http.Client, channelID string, channelName string, logger *util.Logger, ignoreOlderThan time.Duration) (LiveInfo, error) {
	logger.Debug("YouTubeAPI", fmt.Sprintf("Checking channel %s (%s) for recent videos", channelName, channelID))
	return CheckYouTubeViaRSS(httpClient, channelID, channelName, logger, ignoreOlderThan)
}

// CheckYouTubeViaLivePage performs a check by navigating to the channel's /live endpoint.
// YouTube redirects this URL to the active livestream if one exists.
// We inspect the resulting page for "isLive":true markers to distinguish actual streams from VOD redirects.
func CheckYouTubeViaLivePage(httpClient *http.Client, channelID string, channelName string, logger *util.Logger) (LiveInfo, error) {
	liveURL := fmt.Sprintf("https://www.youtube.com/channel/%s/live", channelID)
	logger.Debug("YouTubeAPI", fmt.Sprintf("Checking /live endpoint for %s (%s): %s", channelName, channelID, liveURL))

	req, err := http.NewRequest("GET", liveURL, nil)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create /live request for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
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
		errorMsg := fmt.Sprintf("Failed to fetch /live for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorMsg := fmt.Sprintf("/live request returned non-200 status: %s", resp.Status)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Read body (limit to 1MB to capture the scripts containing ytInitialPlayerResponse)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read /live body for %s: %v", channelName, err)
		logger.Debug("YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
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
		logger.Debug("YouTubeAPI", fmt.Sprintf("/live redirect for %s is a scheduled event. Starts: %s (in %v)", channelName, scheduledTime, timeUntil))

		// If it's scheduled but not explicitly LIVE yet, treat it as offline.
		// This filters out "glorified chatrooms" (streams scheduled far in the future).
		if !isStatusLive {
			return LiveInfo{IsLive: false}, nil
		}
	}

	// Fallback: If not scheduled, check for generic isLive flag.
	// We only use this loose check if we didn't find specific scheduling info to avoid false positives on upcoming events.
	isLiveLoose := strings.Contains(body, `"isLive":true`)

	if !isStatusLive && !isLiveLoose {
		logger.Debug("YouTubeAPI", fmt.Sprintf("%s /live page did not contain live indicators (likely redirect to VOD or channel home).", channelName))
		return LiveInfo{IsLive: false}, nil
	}

	// Extract Video ID
	videoIDRegex := regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
	videoIDMatch := videoIDRegex.FindStringSubmatch(body)
	if len(videoIDMatch) < 2 {
		logger.Debug("YouTubeAPI", fmt.Sprintf("Detected live status for %s but could not extract Video ID.", channelName))
		return LiveInfo{IsLive: false}, nil
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

	return LiveInfo{
		IsLive:    true,
		VideoID:   videoID,
		Title:     title,
		CreatedAt: time.Now(), // Approximate since we don't parse the exact start time
	}, nil
}
