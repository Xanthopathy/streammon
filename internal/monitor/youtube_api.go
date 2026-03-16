package monitor

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"streammon/internal/config"
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

// CheckYouTubeViaRSS parses the YouTube RSS feed for a channel to detect recent videos.
// It returns LiveInfo with the latest recent video that matches the age filter.
// Unlike strict "live" detection, this approach is simpler and more reliable:
// - Just checks if video's updated timestamp is recent (within ignore_older_than)
// - Lets yt-dlp determine if it's actually a live stream
// This matches the approach used by hoshinova for better reliability
func CheckYouTubeViaRSS(httpClient *http.Client, channelID string, globalCfg *config.GlobalConfig, ignoreOlderThan time.Duration) (LiveInfo, error) {
	rssURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("Fetching RSS feed for %s: %s", channelID, rssURL))

	resp, err := httpClient.Get(rssURL)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to fetch RSS feed for %s: %v", channelID, err)
		util.DebugLog(globalCfg, "YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorMsg := fmt.Sprintf("RSS feed request returned non-200 status: %s", resp.Status)
		util.DebugLog(globalCfg, "YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read RSS feed for %s: %v", channelID, err)
		util.DebugLog(globalCfg, "YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("RSS feed for %s (first 1000 chars): %s", channelID, string(body[:min(1000, len(body))])))

	// Parse the RSS feed
	var feed YouTubeRSSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		errorMsg := fmt.Sprintf("Failed to parse RSS feed for %s: %v", channelID, err)
		util.DebugLog(globalCfg, "YouTubeAPI", errorMsg)
		return LiveInfo{}, fmt.Errorf("%s", errorMsg)
	}

	// Look for the latest video that's recent enough
	// Matches hoshinova's approach: just check Updated timestamp, let yt-dlp figure out if it's live
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
			util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("Skipping %s: too old (Updated=%s < cutoff=%s)", entry.VideoID, timestampToCheck, cutoff))
			continue
		}

		// Found a recent video that yt-dlp can try to download
		if entry.VideoID != "" {
			util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("Found recent video from RSS: %s (Published=%s, Updated=%s, Title=%s)", entry.VideoID, entry.Published, entry.Updated, entry.Title))
			return LiveInfo{
				IsLive:    true, // Mark as "live" for processing, yt-dlp will determine actual status
				VideoID:   entry.VideoID,
				Title:     entry.Title,
				CreatedAt: entry.Updated,
			}, nil
		}
	}

	util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("No recent videos found in RSS feed for %s (cutoff=%s)", channelID, cutoff))
	return LiveInfo{IsLive: false}, nil
}

// CheckLiveYouTube checks if a channel has recent videos worth downloading.
// It uses a simple, reliable approach matching hoshinova:
// 1. Fetch the RSS feed
// 2. Check if the latest video's Updated timestamp is recent (within ignore_older_than)
// 3. Return the video details - let yt-dlp determine if it's actually a live stream
// This avoids issues with strict live-detection methods and rate limiting.
func CheckLiveYouTube(httpClient *http.Client, channelID string, globalCfg *config.GlobalConfig, ignoreOlderThan time.Duration) (LiveInfo, error) {
	util.DebugLog(globalCfg, "YouTubeAPI", fmt.Sprintf("Checking channel %s for recent videos", channelID))
	return CheckYouTubeViaRSS(httpClient, channelID, globalCfg, ignoreOlderThan)
}
