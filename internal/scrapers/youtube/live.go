package youtube

import (
	"context"
	"encoding/json"
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

var (
	canonicalVideoRegex          = regexp.MustCompile(`<link rel="canonical" href="https://www.youtube.com/watch\?v=([a-zA-Z0-9_-]{11})">`)
	playerResponseScriptRegex    = regexp.MustCompile(`(?s)ytInitialPlayerResponse\s*=\s*(\{.*?\})\s*;`)
	channelIDLooseRegex          = regexp.MustCompile(`"(?:channelId|externalId|ownerDocId)":"(UC[a-zA-Z0-9_-]{22})"`)
	videoIDLooseRegex            = regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
	titleTagRegex                = regexp.MustCompile(`<title>(.*?) - YouTube</title>`)
	scheduledStartTimeLooseRegex = regexp.MustCompile(`"scheduledStartTime":"(\d+)"`)
)

type ytPlayerResponse struct {
	VideoDetails struct {
		VideoID   string `json:"videoId"`
		ChannelID string `json:"channelId"`
		IsLive    bool   `json:"isLive"`
		Title     string `json:"title"`
	} `json:"videoDetails"`
	Microformat struct {
		PlayerMicroformatRenderer struct {
			LiveBroadcastDetails struct {
				IsLiveNow          bool   `json:"isLiveNow"`
				StartTimestamp     string `json:"startTimestamp"`
				ScheduledStartTime string `json:"scheduledStartTime"`
			} `json:"liveBroadcastDetails"`
		} `json:"playerMicroformatRenderer"`
	} `json:"microformat"`
	PlayabilityStatus struct {
		LiveStreamability interface{} `json:"liveStreamability"`
		Status            string      `json:"status"`
	} `json:"playabilityStatus"`
}

type livePageEvaluation struct {
	isLive        bool
	videoID       string
	title         string
	scheduledTime time.Time
	isScheduled   bool
	hasOwnerMatch bool
}

func evaluateLivePageBody(body string, channelID string) livePageEvaluation {
	eval := livePageEvaluation{}

	canonicalMatch := canonicalVideoRegex.FindStringSubmatch(body)
	if len(canonicalMatch) >= 2 {
		eval.videoID = canonicalMatch[1]
	}

	playerMatch := playerResponseScriptRegex.FindStringSubmatch(body)
	if len(playerMatch) >= 2 {
		var pr ytPlayerResponse
		if err := json.Unmarshal([]byte(playerMatch[1]), &pr); err == nil {
			ownerID := pr.VideoDetails.ChannelID
			if ownerID != "" && ownerID == channelID {
				eval.hasOwnerMatch = true
			}

			if eval.videoID != "" && pr.VideoDetails.VideoID != "" && pr.VideoDetails.VideoID != eval.videoID {
				return eval
			}

			if eval.videoID == "" && pr.VideoDetails.VideoID != "" {
				eval.videoID = pr.VideoDetails.VideoID
			}

			micro := pr.Microformat.PlayerMicroformatRenderer.LiveBroadcastDetails
			if micro.ScheduledStartTime != "" {
				if ts, err := strconv.ParseInt(micro.ScheduledStartTime, 10, 64); err == nil {
					eval.scheduledTime = time.Unix(ts, 0)
					eval.isScheduled = true
				}
			}

			if pr.VideoDetails.Title != "" {
				eval.title = pr.VideoDetails.Title
			}

			isLiveStructured := pr.VideoDetails.IsLive || micro.IsLiveNow || pr.PlayabilityStatus.LiveStreamability != nil
			eval.isLive = eval.hasOwnerMatch && eval.videoID != "" && isLiveStructured
			return eval
		}
	}

	// Conservative fallback when structured payload is unavailable.
	// We require canonical video ID anchoring and owner presence, but avoid global LIVE checks.
	if eval.videoID != "" {
		idPos := strings.Index(body, fmt.Sprintf(`"videoId":"%s"`, eval.videoID))
		if idPos != -1 {
			searchRange := body[idPos:min(idPos+2048, len(body))]
			ownerInRange := strings.Contains(searchRange, fmt.Sprintf(`"channelId":"%s"`, channelID)) || strings.Contains(searchRange, fmt.Sprintf(`"ownerChannelName":"%s"`, channelID))
			if ownerInRange {
				eval.hasOwnerMatch = true
				eval.isLive = strings.Contains(searchRange, `"isLive":true`) || strings.Contains(searchRange, `"isLiveNow":true`) || strings.Contains(searchRange, `"status":"LIVE"`)
			}
		}
	}

	if !eval.hasOwnerMatch {
		for _, match := range channelIDLooseRegex.FindAllStringSubmatch(body, -1) {
			if len(match) >= 2 && match[1] == channelID {
				eval.hasOwnerMatch = true
				break
			}
		}
	}

	if eval.videoID == "" {
		videoIDMatch := videoIDLooseRegex.FindStringSubmatch(body)
		if len(videoIDMatch) >= 2 {
			eval.videoID = videoIDMatch[1]
		}
	}

	if len(eval.title) == 0 {
		titleMatch := titleTagRegex.FindStringSubmatch(body)
		if len(titleMatch) >= 2 {
			eval.title = titleMatch[1]
		}
	}

	if !eval.isScheduled {
		scheduledMatch := scheduledStartTimeLooseRegex.FindStringSubmatch(body)
		if len(scheduledMatch) >= 2 {
			if ts, err := strconv.ParseInt(scheduledMatch[1], 10, 64); err == nil {
				eval.scheduledTime = time.Unix(ts, 0)
				eval.isScheduled = true
			}
		}
	}

	return eval
}

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

	eval := evaluateLivePageBody(body, channelID)

	if !eval.hasOwnerMatch {
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("%s%s%s /live page did not contain the expected channel ID %s. Likely redirect to suggestion or featured channel.", ansi.ColorOrange, channelName, ansi.ColorReset, channelID))
		return models.LiveInfo{IsLive: false}, nil
	}

	if eval.isScheduled {
		timeUntil := time.Until(eval.scheduledTime)
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("/live redirect for %s%s%s is a scheduled event. Starts: %s (in %v)", ansi.ColorOrange, channelName, ansi.ColorReset, eval.scheduledTime, timeUntil))

		// If it's scheduled but not explicitly LIVE yet, treat it as offline.
		// This filters out "glorified chatrooms" (streams scheduled far in the future).
		if !eval.isLive {
			return models.LiveInfo{IsLive: false}, nil
		}
	}

	if !eval.isLive {
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("%s%s%s /live page did not contain live indicators (likely redirect to VOD or channel home).", ansi.ColorOrange, channelName, ansi.ColorReset))
		return models.LiveInfo{IsLive: false}, nil
	}

	if eval.videoID == "" {
		logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Detected live status for %s%s%s but could not extract Video ID.", ansi.ColorOrange, channelName, ansi.ColorReset))
		return models.LiveInfo{IsLive: false}, nil
	}

	title := eval.title
	if title == "" {
		title = "Unknown Title"
	}

	logger.Debug(logging.DebugYouTubeAPI, fmt.Sprintf("Found live stream via /live: %s (%s)", title, eval.videoID))

	return models.LiveInfo{
		IsLive:    true,
		VideoID:   eval.videoID,
		Title:     title,
		CreatedAt: time.Now(), // Approximate since we don't parse the exact start time
	}, nil
}
