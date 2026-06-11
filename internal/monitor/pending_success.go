package monitor

import (
	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/ansi"
)

const (
	ytRetryModeAlternate     = "alternate"
	ytRetryModeSameTimestamp = "same_timestamp"
	ytRetryModeOfflineVOD    = "offline_vod"
)

type pendingYTSuccess struct {
	videoID                           string
	source                            string
	completedPoll                     uint64
	completedDownloader               string
	confirmedStillLiveAfterCompletion bool
}

type pendingSuccess struct {
	videoID       string
	completedPoll uint64
	downloader    string
}

type ytRetryDownloader struct {
	videoID             string
	mode                string
	completedDownloader string
}

func (b *BaseMonitor) setPendingYTSuccess(channelID, videoID string, source string, downloaderName string) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	b.pendingYTSuccesses[channelID] = pendingYTSuccess{
		videoID:             videoID,
		source:              source,
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

func (b *BaseMonitor) takeYTRetryDownloader(channelID, videoID string) (ytRetryDownloader, bool) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	retry, ok := b.ytRetryDownloaders[channelID]
	if !ok || retry.videoID != videoID {
		return ytRetryDownloader{}, false
	}

	delete(b.ytRetryDownloaders, channelID)
	return retry, true
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
			retry := ytRetryDownloader{
				videoID:             pending.videoID,
				mode:                ytRetryModeAlternate,
				completedDownloader: pending.completedDownloader,
			}
			_, downloaderName, enabled := controller.BuildRetryDownloaderCmd(ch, newStatus, retry)
			if enabled && downloaderName != "" {
				delete(b.pendingYTSuccesses, ch.ID)
				b.ytRetryDownloaders[ch.ID] = retry
				b.pendingYTSuccessMu.Unlock()

				b.logger.LogEventf("RETRY", "%s%s%s (%s) is still live after %s completed. Retrying with %s.",
					ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.completedDownloader, downloaderName)
				return
			}

			retry = ytRetryDownloader{
				videoID:             pending.videoID,
				mode:                ytRetryModeSameTimestamp,
				completedDownloader: pending.completedDownloader,
			}
			_, downloaderName, enabled = controller.BuildRetryDownloaderCmd(ch, newStatus, retry)
			if enabled && downloaderName != "" {
				delete(b.pendingYTSuccesses, ch.ID)
				b.ytRetryDownloaders[ch.ID] = retry
				b.pendingYTSuccessMu.Unlock()

				b.logger.LogEventf("RETRY", "%s%s%s (%s) is still live after %s completed. Retrying %s with a timestamped output name.",
					ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.completedDownloader, downloaderName)
				return
			}
		}

		pending.completedPoll = pollID
		pending.confirmedStillLiveAfterCompletion = true
		b.pendingYTSuccesses[ch.ID] = pending
		b.pendingYTSuccessMu.Unlock()

		b.logger.LogEventf("ARCHIVE", "%s%s%s (%s) is still live after %s completed. Waiting for it to go offline; no alternate downloader is enabled.",
			ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.completedDownloader)
		return
	}

	delete(b.pendingYTSuccesses, ch.ID)
	b.pendingYTSuccessMu.Unlock()

	retryStatus := newStatus
	retryStatus.VideoID = pending.videoID
	retryStatus.Source = pending.source

	if pending.confirmedStillLiveAfterCompletion {
		if controller, canRetry := b.controller.(RetryDownloaderController); canRetry {
			retry := ytRetryDownloader{
				videoID:             pending.videoID,
				mode:                ytRetryModeOfflineVOD,
				completedDownloader: pending.completedDownloader,
			}
			if _, downloaderName, enabled := controller.BuildRetryDownloaderCmd(ch, retryStatus, retry); enabled && downloaderName != "" {
				b.pendingYTSuccessMu.Lock()
				b.ytRetryDownloaders[ch.ID] = retry
				b.pendingYTSuccessMu.Unlock()

				b.logger.LogEventf("RETRY", "%s%s%s (%s) is no longer live. Retrying final VOD with %s and live-wait args removed.",
					ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, downloaderName)
				if b.tryStartDownload(ch, retryStatus) {
					return
				}

				b.pendingYTSuccessMu.Lock()
				delete(b.ytRetryDownloaders, ch.ID)
				pending.completedPoll = pollID
				b.pendingYTSuccesses[ch.ID] = pending
				b.pendingYTSuccessMu.Unlock()

				b.logger.LogEventf("RETRY", "%s%s%s (%s) final VOD retry could not start yet. Keeping pending success for the next poll.",
					ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID)
				return
			}
		}
	}

	b.logger.LogEventf("ARCHIVE", "%s%s%s (%s) is no longer live. Archiving completed YouTube download.",
		ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID)
	b.finalizeSuccessfulDownload(ch.ID, pending.videoID, b.logger)
}

func (b *BaseMonitor) setPendingTwitchSuccess(channelID, videoID string, downloaderName string) {
	b.pendingYTSuccessMu.Lock()
	defer b.pendingYTSuccessMu.Unlock()

	b.pendingTwitchSuccesses[channelID] = pendingSuccess{
		videoID:       videoID,
		completedPoll: b.pollGeneration.Load(),
		downloader:    downloaderName,
	}
}

func (b *BaseMonitor) resolvePendingTwitchSuccess(ch config.Channel, pollID uint64) {
	b.pendingYTSuccessMu.Lock()
	pending, ok := b.pendingTwitchSuccesses[ch.ID]
	if !ok || pollID <= pending.completedPoll {
		b.pendingYTSuccessMu.Unlock()
		return
	}

	delete(b.pendingTwitchSuccesses, ch.ID)
	b.pendingYTSuccessMu.Unlock()

	b.logger.LogEventf("ARCHIVE", "%s%s%s (%s) completed with %s on the previous poll. Archiving Twitch download.",
		ansi.ColorOrange, ch.Name, ansi.ColorReset, pending.videoID, pending.downloader)
	b.finalizeSuccessfulDownload(ch.ID, pending.videoID, b.logger)
}
