package monitor

import (
	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/ansi"
)

type pendingYTSuccess struct {
	videoID       string
	completedPoll uint64
}

func (b *BaseMonitor) setPendingYTSuccess(channelID, videoID string) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	b.pendingYTSuccesses[channelID] = pendingYTSuccess{
		videoID:       videoID,
		completedPoll: b.pollGeneration.Load(),
	}
}

func (b *BaseMonitor) hasPendingYTSuccess(channelID, videoID string) bool {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	pending, ok := b.pendingYTSuccesses[channelID]
	return ok && pending.videoID == videoID
}

func (b *BaseMonitor) takePendingYTSuccess(channelID string, pollID uint64) (pendingYTSuccess, bool) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	pending, ok := b.pendingYTSuccesses[channelID]
	if !ok || pollID <= pending.completedPoll {
		return pendingYTSuccess{}, false
	}

	delete(b.pendingYTSuccesses, channelID)
	return pending, true
}

func (b *BaseMonitor) resolvePendingYTSuccess(ch config.Channel, newStatus models.LiveInfo, pollID uint64) {
	if pending, ok := b.takePendingYTSuccess(ch.ID, pollID); ok {
		if newStatus.IsLive && newStatus.VideoID == pending.videoID {
			// A completed yt-dlp output may already exist here. yt-dlp is expected to handle the repeated invocation cleanly; maybe verify with a real long stream.
			b.logger.Logf("%s%s%s (%s) is still live after downloader completion. Allowing another download attempt.", ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID)
		} else {
			b.finalizeSuccessfulDownload(ch.ID, pending.videoID, b.logger)
		}
	}
}
