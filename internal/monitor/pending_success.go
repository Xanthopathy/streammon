package monitor

import (
	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/ansi"
)

type pendingYTSuccess struct {
	videoID             string
	completedPoll       uint64
	completedDownloader string
}

type ytRetryDownloader struct {
	videoID         string
	avoidDownloader string
}

func (b *BaseMonitor) setPendingYTSuccess(channelID, videoID, downloaderName string) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	b.pendingYTSuccesses[channelID] = pendingYTSuccess{
		videoID:             videoID,
		completedPoll:       b.pollGeneration.Load(),
		completedDownloader: downloaderName,
	}
}

func (b *BaseMonitor) hasPendingYTSuccess(channelID, videoID string) bool {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	pending, ok := b.pendingYTSuccesses[channelID]
	return ok && pending.videoID == videoID
}

func (b *BaseMonitor) takeYTRetryDownloader(channelID, videoID string) (string, bool) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	retry, ok := b.ytRetryDownloaders[channelID]
	if !ok || retry.videoID != videoID {
		return "", false
	}

	delete(b.ytRetryDownloaders, channelID)
	return retry.avoidDownloader, true
}

func (b *BaseMonitor) resolvePendingYTSuccess(ch config.Channel, newStatus models.LiveInfo, pollID uint64) {
	b.pendingYTSuccessMu.Lock()
	pending, ok := b.pendingYTSuccesses[ch.ID]
	if !ok || pollID <= pending.completedPoll {
		b.pendingYTSuccessMu.Unlock()
		return
	}

	if newStatus.IsLive && newStatus.VideoID == pending.videoID {
		controller, canRetry := b.controller.(RetryDownloaderController)
		if canRetry {
			_, downloaderName, enabled := controller.BuildRetryDownloaderCmd(ch, newStatus, pending.completedDownloader)
			if enabled && downloaderName != "" {
				delete(b.pendingYTSuccesses, ch.ID)
				b.ytRetryDownloaders[ch.ID] = ytRetryDownloader{
					videoID:         pending.videoID,
					avoidDownloader: pending.completedDownloader,
				}
				b.pendingYTSuccessMu.Unlock()

				b.logger.Logf("%s%s%s (%s) is still live after %s completed. Retrying with %s.",
					ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.completedDownloader, downloaderName)
				return
			}
		}

		pending.completedPoll = pollID
		b.pendingYTSuccesses[ch.ID] = pending
		b.pendingYTSuccessMu.Unlock()

		b.logger.Logf("%s%s%s (%s) is still live after %s completed. Waiting for it to go offline; no alternate downloader is enabled.",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.completedDownloader)
		return
	}

	delete(b.pendingYTSuccesses, ch.ID)
	b.pendingYTSuccessMu.Unlock()

	b.logger.Logf("%s%s%s (%s) is no longer live. Archiving completed YouTube download.",
		ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID)
	b.finalizeSuccessfulDownload(ch.ID, pending.videoID, b.logger)
}
