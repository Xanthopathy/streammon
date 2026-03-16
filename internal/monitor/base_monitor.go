package monitor

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// downloadProcess holds information about a running download process.
type downloadProcess struct {
	cmd      *exec.Cmd
	videoID  string
	lockPath string
	logFile  *os.File
}

// MonitorController defines the platform-specific logic that a monitor must implement.
type MonitorController interface {
	// Getters for configuration and identity
	GetGlobalConfig() *config.GlobalConfig
	GetStreamMonConfig() *config.StreamMonConfig
	GetChannels() []config.Channel
	GetPollInterval() (time.Duration, error)
	GetLogColor() string
	GetLogPrefix() string

	// Core platform-specific logic
	CheckChannelStatus(ch config.Channel, httpClient *http.Client) (LiveInfo, error)
	BuildDownloaderCmd(ch config.Channel, status LiveInfo) *exec.Cmd
}

// BaseMonitor provides the generic, shared functionality for monitoring any platform.
type BaseMonitor struct {
	controller      MonitorController
	httpClient      *http.Client
	statusMutex     sync.RWMutex
	downloadMutex   sync.Mutex
	liveStatus      map[string]LiveInfo         // map[channelID]LiveInfo
	activeDownloads map[string]*downloadProcess // map[channelID]*downloadProcess
}

// NewBaseMonitor creates a new generic monitor.
func NewBaseMonitor(controller MonitorController) *BaseMonitor {
	return &BaseMonitor{
		controller:      controller,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		liveStatus:      make(map[string]LiveInfo),
		activeDownloads: make(map[string]*downloadProcess),
	}
}

// Run starts the main monitoring loop.
func (b *BaseMonitor) Run() {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	channels := b.controller.GetChannels()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	fmt.Printf("%s [%s%s%s] Monitor started for %d channels.\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, len(channels))
	fmt.Printf("%s [%s%s%s] Working Directory: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, streamMonCfg.WorkingDirectory)

	// Create working directory if it doesn't exist
	if _, err := os.Stat(streamMonCfg.WorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(streamMonCfg.WorkingDirectory, 0755)
		if err != nil {
			fmt.Printf("%s [%s%s%s] Error creating working directory: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, err)
			return
		}
		fmt.Printf("%s [%s%s%s] Created working directory: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, streamMonCfg.WorkingDirectory)
	}
	// Start the download manager in the background
	go b.manageDownloads()

	// Configure the main check ticker
	pollInterval, err := b.controller.GetPollInterval()
	if err != nil {
		fmt.Printf("%s [%s%s%s] Invalid poll_interval, defaulting to 60s. Error: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, err)
		pollInterval = 60 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Run initial check immediately
	b.checkAllChannels()

	for range ticker.C {
		b.checkAllChannels() // Then check on every tick
	}
}

// manageDownloads is a loop that periodically checks for live channels that need downloading.
func (b *BaseMonitor) manageDownloads() {
	managerInterval := 5 * time.Second
	for {
		time.Sleep(managerInterval)

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
	b.downloadMutex.Lock()
	defer b.downloadMutex.Unlock()

	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()

	// 1. Check concurrency
	if len(b.activeDownloads) >= globalCfg.MaxConcurrentDownloads {
		return // At capacity
	}

	// 2. Check if already downloading
	if _, exists := b.activeDownloads[ch.ID]; exists {
		return
	}

	// 3. Check for lock file (in case of restart)
	lockPath := util.GetLockfilePath(streamMonCfg.WorkingDirectory, ch.Name, status.VideoID)
	if util.HasLock(lockPath) {
		return
	}

	// All clear, launch it.
	b.launchDownloader(ch, status, lockPath)
}

// launchDownloader creates a lockfile and starts the downloader subprocess.
// This function must be called with the downloadMutex held.
func (b *BaseMonitor) launchDownloader(ch config.Channel, status LiveInfo, lockPath string) {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	// Create lockfile
	if err := util.CreateLock(lockPath); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating lockfile for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return
	}

	// Build command using the controller
	cmd := b.controller.BuildDownloaderCmd(ch, status)

	// Create channel specific directory
	channelDir := filepath.Join(streamMonCfg.WorkingDirectory, util.SanitizeFolderName(ch.Name))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating directory for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return
	}
	cmd.Dir = channelDir

	// Setup logging if enabled
	var logFile *os.File
	if globalCfg.SaveDownloadLogs {
		logName := fmt.Sprintf("%s_[%s].log", status.CreatedAt.UTC().Format("2006-01-02"), status.VideoID)
		f, err := os.Create(filepath.Join(channelDir, logName))
		if err == nil {
			logFile = f
			cmd.Stdout = f
			cmd.Stderr = f
		} else {
			fmt.Printf("%s [%s%s%s] Failed to create log file: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, err)
		}
	}

	// Start command
	if err := cmd.Start(); err != nil {
		fmt.Printf("%s [%s%s%s] Error starting download for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath) // Clean up lock on failure
		if logFile != nil {
			logFile.Close()
		}
		return
	}

	fmt.Printf("%s [%s%s%s] %sStarted download for %s%s: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, util.ColorGreen, ch.Name, util.ColorReset, status.Title)

	// Store process info
	proc := &downloadProcess{
		cmd:      cmd,
		videoID:  status.VideoID,
		lockPath: lockPath,
		logFile:  logFile,
	}
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
}

// waitForDownload blocks until a download process finishes, then cleans up.
func (b *BaseMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Now clean up
	b.downloadMutex.Lock()
	delete(b.activeDownloads, ch.ID)
	b.downloadMutex.Unlock()

	util.DeleteLock(proc.lockPath)

	if proc.logFile != nil {
		proc.logFile.Close()
	}

	globalCfg := b.controller.GetGlobalConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	if err != nil {
		fmt.Printf("%s [%s%s%s] Download for %s finished with error: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
	} else {
		fmt.Printf("%s [%s%s%s] Download for %s finished successfully.\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name)
	}
}

// checkAllChannels concurrently checks all configured channels.
func (b *BaseMonitor) checkAllChannels() {
	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()
	channels := b.controller.GetChannels()

	util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("Checking live status for %d channels...", len(channels)))

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go b.checkChannel(ch, &wg)
	}
	wg.Wait()
}

// checkChannel is the core logic for checking a single channel's status.
func (b *BaseMonitor) checkChannel(ch config.Channel, wg *sync.WaitGroup) {
	defer wg.Done()

	globalCfg := b.controller.GetGlobalConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	newStatus, err := b.controller.CheckChannelStatus(ch, b.httpClient)
	if err != nil {
		fmt.Printf("%s [%s%s%s] Error checking %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return
	}

	// --- SAFETY NET LOGIC (pre-lock check) ---
	if !newStatus.IsLive {
		b.statusMutex.RLock()
		previousStatus, wasTracked := b.liveStatus[ch.ID]
		b.statusMutex.RUnlock()

		b.downloadMutex.Lock()
		proc, isDownloading := b.activeDownloads[ch.ID]
		b.downloadMutex.Unlock()

		if wasTracked && previousStatus.IsLive && isDownloading && proc.videoID == newStatus.LastBroadcastID {
			util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("API reports %s as offline, but download is active for same stream ID (%s). Ignoring.", ch.Name, proc.videoID))
			return // Ignore this offline signal.
		}
	}
	// --- END SAFETY NET ---

	b.statusMutex.Lock()
	defer b.statusMutex.Unlock()

	previousStatus, wasTracked := b.liveStatus[ch.ID]

	// Handle state changes
	if newStatus.IsLive {
		// Filter check
		matchesFilter := false
		if len(ch.Filters) == 0 { // If no filters, always match
			matchesFilter = true
		} else {
			for _, filter := range ch.Filters {
				if matched, _ := regexp.MatchString(filter, newStatus.Title); matched {
					matchesFilter = true
					break
				}
			}
		}

		if !matchesFilter {
			if wasTracked && previousStatus.IsLive {
				fmt.Printf("%s [%s%s%s] %s is live but filtered out: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, newStatus.Title)
				b.liveStatus[ch.ID] = LiveInfo{IsLive: false}
			}
			return
		}

		if !wasTracked || !previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s%s is now LIVE%s: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, util.ColorGreen, ch.Name, util.ColorReset, newStatus.Title)
		}
		b.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline (genuine case, safety net already passed)
		if wasTracked && previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s%s has gone OFFLINE%s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, util.ColorRed, ch.Name, util.ColorReset)
		}
		b.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
}
