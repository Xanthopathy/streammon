package monitor

import (
	"fmt"
	"streammon/internal/util/fileio"
	"streammon/internal/util/logging"
	"strings"
	"time"
)

func mediaFileMatchesDownload(name string, modTime time.Time, proc *downloadProcess) bool {
	if !isMediaFile(name) {
		return false
	}

	switch proc.downloaderName {
	case "yt-dlp":
		if strings.Contains(name, proc.videoID) {
			return true
		}
		if len(proc.videoID) >= 8 && strings.Contains(name, proc.videoID[:8]) {
			return true
		}
		return false
	case "twitch-dlp":
		// twitch-dlp's %(id)s can be a VOD-style ID (e.g. v2782168798), while
		// streammon tracks the live GQL stream ID. Use files touched by this run.
		return !modTime.Before(proc.startedAt.Add(-10 * time.Second))
	default:
		return strings.Contains(name, proc.videoID) || !modTime.Before(proc.startedAt.Add(-10*time.Second))
	}
}

func isMediaFile(name string) bool {
	return strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mkv") || strings.HasSuffix(name, ".webm")
}

func (b *BaseMonitor) finalizeSuccessfulDownload(channelID string, videoID string, logger *logging.Logger) {
	b.downloadedVidMu.Lock()
	if _, ok := b.downloadedVideos[channelID]; !ok {
		b.downloadedVideos[channelID] = make(map[string]bool)
	}
	b.downloadedVideos[channelID][videoID] = true
	b.downloadedVidMu.Unlock()

	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()

	shouldArchive := (logPrefix == logPrefixYouTube && globalCfg.YoutubeArchiveDownloads) || (logPrefix == logPrefixTwitch && globalCfg.TwitchArchiveDownloads)

	if !shouldArchive {
		return
	}

	archivePath := b.archivePath()

	if err := fileio.AppendLineToFile(archivePath, videoID); err != nil {
		logger.LogError(fmt.Sprintf("Failed to archive video ID: %v", err))
		return
	}

	b.archivedVidMu.Lock()
	b.archivedVideos[videoID] = true
	b.archivedVidMu.Unlock()
}
