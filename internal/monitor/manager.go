package monitor

import (
	"fmt"
	"path/filepath"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// manageDownloads is a loop that periodically checks for live channels that need downloading.
func (b *BaseMonitor) manageDownloads() {
	managerInterval := 5 * time.Second
	for {
		time.Sleep(managerInterval)

		// Periodically reset terminal title to prevent subprocesses from changing it
		util.SetTerminalTitle("streammon")

		b.statusMutex.RLock()
		// Create a copy of live channels to avoid holding the lock for too long
		liveChs := make(map[string]LiveInfo)
		for id, s := range b.liveStatus {
			if s.IsLive {
				liveChs[id] = s
			}
		}
		b.statusMutex.RUnlock()

		// Iterate in config order for priority
		for _, ch := range b.controller.GetChannels() {
			status, isLive := liveChs[ch.ID]
			if !isLive {
				continue
			}
			// Try to start a download. The function will handle all checks.
			b.tryStartDownload(ch, status)
		}
	}
}

// tryStartDownload checks all conditions and launches a download if appropriate.
func (b *BaseMonitor) tryStartDownload(ch config.Channel, status LiveInfo) {
	// 1. Try to acquire a global download slot. This is non-blocking.
	select {
	case downloadSlots <- struct{}{}:
		// Slot acquired. We are now responsible for releasing it on any failure.
	default:
		return // Global capacity reached.
	}

	// If we return from now on, we must release the slot.
	// A defer with a flag is a robust way to handle this.
	var launchOK bool
	defer func() {
		if !launchOK {
			<-downloadSlots // Release slot on any failure path.
		}
	}()

	// 2. Perform all pre-flight checks under a lock.
	b.downloadMutex.Lock()
	defer b.downloadMutex.Unlock()

	// Check if already downloading in this monitor instance.
	if _, exists := b.activeDownloads[ch.ID]; exists {
		return // Defer will release slot.
	}

	// Check if already downloaded in this session (in-memory cache).
	var alreadyDownloaded bool
	b.downloadedVidMu.RLock()
	if channelCache, ok := b.downloadedVideos[ch.ID]; ok {
		alreadyDownloaded = channelCache[status.VideoID]
	}
	b.downloadedVidMu.RUnlock()

	// Check archive
	b.archivedVidMu.RLock()
	isArchived := b.archivedVideos[status.VideoID]
	b.archivedVidMu.RUnlock()

	if alreadyDownloaded || isArchived {
		// Only log this message once per video to avoid spam
		b.downloadedVidsLoggedMutex.Lock()
		if !b.downloadedVidsLogged[status.VideoID] {
			b.downloadedVidsLogged[status.VideoID] = true
			globalCfg := b.controller.GetGlobalConfig()
			logColor := b.controller.GetLogColor()
			logPrefix := b.controller.GetLogPrefix()
			reason := "already downloaded in this session"
			if isArchived {
				reason = "found in archive"
			}
			fmt.Printf("%s [%s%s%s] %s (%s) skipped: %s\n",
				util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, status.VideoID, reason)
		}
		b.downloadedVidsLoggedMutex.Unlock()
		return // Defer will release slot.
	}

	// Check for a lock file.
	streamMonCfg := b.controller.GetStreamMonConfig()
	lockPath := util.GetLockfilePath(streamMonCfg.WorkingDirectory, ch.Name, status.VideoID)
	if util.HasLock(lockPath) {
		// Only log this message once per video to avoid spam
		b.queuedVideosLoggedMutex.Lock()
		if !b.queuedVideosLogged[status.VideoID] {
			b.queuedVideosLogged[status.VideoID] = true
			globalCfg := b.controller.GetGlobalConfig()
			logColor := b.controller.GetLogColor()
			logPrefix := b.controller.GetLogPrefix()
			lockFileName := filepath.Base(lockPath)
			fmt.Printf("%s [%s%s%s] %s (%s) is already queued/downloading (lockfile exists). If restarting, remove: %s\n",
				util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, status.VideoID, lockFileName)
		}
		b.queuedVideosLoggedMutex.Unlock()
		return // Defer will release slot.
	}

	// 3. All checks passed. Launch the downloader.
	// If launch is successful, it becomes responsible for the slot.
	if b.launchDownloader(ch, status, lockPath) {
		launchOK = true // Success! The defer will NOT release the slot.
	}
}
