package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"streammon/internal/models"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

type memberPlaylistInfo struct {
	Entries []memberPlaylistEntry `json:"entries"`
}

type memberPlaylistEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	LiveStatus string `json:"live_status"`
}

func MembersPlaylistID(channelID string) (string, bool) {
	if !strings.HasPrefix(channelID, "UC") || len(channelID) <= 2 {
		return "", false
	}
	return "UUMO" + strings.TrimPrefix(channelID, "UC"), true
}

func CheckYouTubeViaMembersPlaylist(
	ctx context.Context,
	cookiesFile string,
	memberCheckArgs []string,
	channelID string,
	channelName string,
	logger *logging.Logger,
) (models.LiveInfo, error) {
	playlistID, ok := MembersPlaylistID(channelID)
	if !ok {
		return models.LiveInfo{IsLive: false}, nil
	}

	playlistURL := "https://www.youtube.com/playlist?list=" + playlistID
	logger.Debug(
		"YouTubeAPI",
		fmt.Sprintf(
			"Checking members playlist for %s%s%s: %s",
			ansi.ColorOrange,
			channelName,
			ansi.ColorReset,
			playlistURL,
		),
	)

	args := []string{}
	if strings.TrimSpace(cookiesFile) != "" {
		args = append(args, "--cookies", cookiesFile)
	}
	args = append(args, memberCheckArgs...)
	args = append(args, playlistURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return models.LiveInfo{}, fmt.Errorf("yt-dlp member playlist check failed: %w", err)
	}

	var playlist memberPlaylistInfo
	if err := json.Unmarshal(out, &playlist); err != nil {
		return models.LiveInfo{}, fmt.Errorf("parse member playlist JSON: %w", err)
	}

	for _, entry := range playlist.Entries {
		if entry.ID == "" {
			continue
		}

		if entry.LiveStatus != "is_live" {
			logger.Debug(
				"YouTubeAPI",
				fmt.Sprintf(
					"Members playlist candidate for %s%s%s is not live: %s (%s, live_status=%s)",
					ansi.ColorOrange,
					channelName,
					ansi.ColorReset,
					entry.Title,
					entry.ID,
					entry.LiveStatus,
				),
			)
			continue
		}

		return models.LiveInfo{
			IsLive:    true,
			VideoID:   entry.ID,
			Title:     entry.Title,
			CreatedAt: time.Now(),
		}, nil
	}

	return models.LiveInfo{IsLive: false}, nil
}
