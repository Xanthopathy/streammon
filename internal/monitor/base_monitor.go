package monitor

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// downloadProcess holds information about a running download process.
type downloadProcess struct {
	cmd       *exec.Cmd
	videoID   string
	lockPath  string
	logger    *util.DownloadLogger
	isWaiting *atomic.Bool // Signals that the process is in a waiting/retry state
}

// --- Global Download Limiter ---

var (
	downloadSlots     chan struct{}
	downloadSlotsOnce sync.Once
)

// initializeDownloadSlots creates the global semaphore for limiting concurrent downloads.
// It's safe to call multiple times; it will only initialize the semaphore once.
func initializeDownloadSlots(max int) {
	downloadSlotsOnce.Do(func() {
		// Ensure at least one download is possible, even if config is 0 or less.
		if max <= 0 {
			max = 1
		}
		downloadSlots = make(chan struct{}, max)
	})
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
	controller                MonitorController
	httpClient                *http.Client
	statusMutex               sync.RWMutex
	downloadMutex             sync.Mutex
	liveStatus                map[string]LiveInfo         // map[channelID]LiveInfo
	activeDownloads           map[string]*downloadProcess // map[channelID]*downloadProcess
	downloadedVideos          map[string]map[string]bool  // map[channelID]map[videoID]bool - in-memory cache of downloaded videos
	downloadedVidMu           sync.RWMutex                // protects downloadedVideos
	queuedVideosLogged        map[string]bool             // map[videoID]bool - tracks which queued videos have logged the "already queued" message
	queuedVideosLoggedMutex   sync.Mutex                  // protects queuedVideosLogged
	downloadedVidsLogged      map[string]bool             // map[videoID]bool - tracks which downloaded videos have logged the "already downloaded" message
	downloadedVidsLoggedMutex sync.Mutex                  // protects downloadedVidsLogged
	archivedVideos            map[string]bool             // map[videoID]bool - loaded from archive.txt
	archivedVidMu             sync.RWMutex                // protects archivedVideos
}

// NewBaseMonitor creates a new generic monitor.
func NewBaseMonitor(controller MonitorController) *BaseMonitor {
	return &BaseMonitor{
		controller:           controller,
		httpClient:           &http.Client{Timeout: 30 * time.Second},
		liveStatus:           make(map[string]LiveInfo),
		activeDownloads:      make(map[string]*downloadProcess),
		downloadedVideos:     make(map[string]map[string]bool),
		queuedVideosLogged:   make(map[string]bool),
		downloadedVidsLogged: make(map[string]bool),
		archivedVideos:       make(map[string]bool),
	}
}

// Run starts the main monitoring loop.
func (b *BaseMonitor) Run() {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	channels := b.controller.GetChannels()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	// Seed random for jitter
	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Initialize the global download semaphore using the value from the global config.
	initializeDownloadSlots(globalCfg.MaxConcurrentDownloads)

	fmt.Printf("[%s%s%s] Monitor started for %d channels.\n", logColor, logPrefix, util.ColorReset, len(channels))
	fmt.Printf("[%s%s%s] Working Directory: %s\n", logColor, logPrefix, util.ColorReset, streamMonCfg.WorkingDirectory)

	// Create working directory if it doesn't exist
	if _, err := os.Stat(streamMonCfg.WorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(streamMonCfg.WorkingDirectory, 0755)
		if err != nil {
			fmt.Printf("%s [%s%s%s] Error creating working directory: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, err)
			return
		}
		fmt.Printf("%s [%s%s%s] Created working directory: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, streamMonCfg.WorkingDirectory)
	}

	// Load archive.txt if enabled to prevent re-downloads
	shouldArchive := false
	if logPrefix == "YT" && globalCfg.YoutubeArchiveDownloads {
		shouldArchive = true
	} else if logPrefix == "Twitch" && globalCfg.TwitchArchiveDownloads {
		shouldArchive = true
	}

	if shouldArchive {
		archivePath := filepath.Join(streamMonCfg.WorkingDirectory, "archive.txt")
		if lines, err := util.ReadLinesToSet(archivePath); err == nil {
			b.archivedVideos = lines
			fmt.Printf("%s [%s%s%s] Loaded %d archived video IDs.\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, len(b.archivedVideos))
		}
	}

	// Start the download manager in the background
	go b.manageDownloads()

	// Configure the main check ticker
	pollInterval, err := b.controller.GetPollInterval()
	if err != nil {
		fmt.Printf("%s [%s%s%s] Invalid poll_interval, defaulting to 60s. Error: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, err)
		pollInterval = 60 * time.Second
	}

	consecutiveErrors := 0

	for {
		// Run check and track errors
		errorCount := b.checkAllChannels()

		// Switch to fixed-delay scheduling aka sleep for the full interval AFTER the work is done.
		// Previously we subtracted work duration, which dangerously reduced quiet time as the channel list grew.
		sleepDuration := pollInterval

		// Add random jitter (-10% to +10%) to the poll interval to mitigate bot pattern recognition
		// Example: 60s becomes something between 54s and 66s.
		jitterPercent := 0.10
		jitterRange := int64(float64(pollInterval) * jitterPercent)
		sleepDuration += time.Duration(rand.Int63n(jitterRange*2) - jitterRange)

		// Backoff logic if errors occurred
		if errorCount > 0 {
			consecutiveErrors++
			// Add 1 minute per consecutive error run, cap at 15 minutes
			backoff := time.Duration(consecutiveErrors) * 1 * time.Minute
			if backoff > 15*time.Minute {
				backoff = 15 * time.Minute
			}
			fmt.Printf("%s [%s%s%s] Detected %d errors during poll. Staggering next poll by +%v (Consecutive failures: %d)\n",
				util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, errorCount, backoff, consecutiveErrors)
			sleepDuration += backoff
		} else {
			consecutiveErrors = 0
		}

		time.Sleep(sleepDuration)
	}
}

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

// launchDownloader creates a lockfile and starts the downloader subprocess.
// This function must be called with the downloadMutex held.
// It returns true on success, false on failure.
func (b *BaseMonitor) launchDownloader(ch config.Channel, status LiveInfo, lockPath string) bool {
	globalCfg := b.controller.GetGlobalConfig()
	streamMonCfg := b.controller.GetStreamMonConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	// Create synchronization for waiting state detection
	isWaiting := &atomic.Bool{}

	// Callback to detect waiting state from subprocess output
	outputCallback := func(line string) {
		if strings.Contains(line, "[retry-streams]") {
			isWaiting.Store(true)
		} else if strings.Contains(line, "frame=") || strings.Contains(line, "[download]") {
			// If we see active download progress, we are no longer waiting.
			isWaiting.Store(false)
		}
	}

	// Create lockfile
	if err := util.CreateLock(lockPath); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating lockfile for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return false
	}

	// Build command using the controller
	cmd := b.controller.BuildDownloaderCmd(ch, status)

	// Build command string for logging
	commandStr := cmd.Path
	if len(cmd.Args) > 1 {
		commandStr += " " + util.JoinCommandArgs(cmd.Args[1:])
	}

	// Create channel specific directory
	channelDir := filepath.Join(streamMonCfg.WorkingDirectory, util.SanitizeFolderName(ch.Name))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		fmt.Printf("%s [%s%s%s] Error creating directory for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return false
	}
	cmd.Dir = channelDir

	// Determine which debug flags to enable based on platform and config
	apiDebug := false
	dlpDebug := false

	switch logPrefix {
	case "Twitch":
		apiDebug = globalCfg.TwitchAPIVerboseDebug
		dlpDebug = globalCfg.TwitchDlpVerboseDebug
	case "YT":
		apiDebug = globalCfg.YoutubeVerboseDebug
		dlpDebug = globalCfg.YoutubeDlpVerboseDebug
	}

	logger, err := util.NewDownloadLogger(
		channelDir,
		ch.ID,
		ch.Name,
		status.VideoID,
		status.CreatedAt,
		globalCfg,
		logPrefix,
		logColor,
		apiDebug,
		dlpDebug,
		commandStr,
	)
	if err != nil {
		fmt.Printf("%s [%s%s%s] Error creating logger for %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return false
	}

	// Confirm dlpDebug setting
	if dlpDebug {
		logger.LogRegular("Raw subprocess output will be shown (dlp_verbose_debug=true)")
	}

	// Force colors in subprocess output (yt-dlp, twitch-dlp)
	// Set environment variables to enable color output even when piping
	// Doesn't work, twitch-dlp already does this and yt-dlp doesn't show color with this
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1", "TERM=xterm-256color")

	// Setup subprocess output redirection
	// Pipe output if we need to log it or show it in terminal (dlpDebug)
	// Determine debugType based on platform prefix
	var debugType string
	switch logPrefix {
	case "YT":
		debugType = "yt-dlp"
	case "Twitch":
		debugType = "twitch-dlp"
	default:
		debugType = "dlp"
	}

	if globalCfg.SaveDownloadLogs || dlpDebug {
		stdoutPipe, errOut := cmd.StdoutPipe()
		stderrPipe, errErr := cmd.StderrPipe()

		if errOut == nil && stdoutPipe != nil {
			go util.ReadPipeAndLog(stdoutPipe, logger, debugType, outputCallback)
		}
		if errErr == nil && stderrPipe != nil {
			go util.ReadPipeAndLog(stderrPipe, logger, debugType, outputCallback)
		}
	}

	// Log the command if dlp debug is enabled (for terminal display)
	if dlpDebug {
		logger.LogSubprocessOutput("COMMAND: "+commandStr, debugType)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		logger.LogError(fmt.Sprintf("Error starting download for %s: %v", ch.Name, err))
		util.DeleteLock(lockPath) // Clean up lock on failure
		logger.Close()
		return false
	}

	logger.LogRegular(fmt.Sprintf("%sStarted download for %s%s: %s", util.ColorGreen, ch.Name, util.ColorReset, status.Title))

	// Store process info
	proc := &downloadProcess{
		cmd:       cmd,
		videoID:   status.VideoID,
		lockPath:  lockPath,
		logger:    logger,
		isWaiting: isWaiting,
	}
	b.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go b.waitForDownload(ch, proc)
	return true
}

// waitForDownload blocks until a download process finishes, then cleans up.
func (b *BaseMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Reset terminal title once subprocess completes
	util.SetTerminalTitle("streammon")

	// IMPORTANT: Release the download slot first thing after the process exits.
	<-downloadSlots

	// Now clean up other resources.
	b.downloadMutex.Lock()
	delete(b.activeDownloads, ch.ID)
	b.downloadMutex.Unlock()

	util.DeleteLock(proc.lockPath)
	globalCfg := b.controller.GetGlobalConfig()
	logPrefix := b.controller.GetLogPrefix()

	util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("Released download slot for %s. Slots used: %d/%d.", ch.Name, len(downloadSlots), cap(downloadSlots)))

	if err != nil {
		proc.logger.LogError(fmt.Sprintf("Download for %s finished with error: %v", ch.Name, err))
	} else {
		proc.logger.LogRegular(fmt.Sprintf("Download for %s finished successfully.", ch.Name))

		// Mark this video as downloaded in the session cache
		b.downloadedVidMu.Lock()
		if _, ok := b.downloadedVideos[ch.ID]; !ok {
			b.downloadedVideos[ch.ID] = make(map[string]bool)
		}
		b.downloadedVideos[ch.ID][proc.videoID] = true
		b.downloadedVidMu.Unlock()

		// Archive the downloaded video ID if enabled
		shouldArchive := false
		if logPrefix == "YT" && globalCfg.YoutubeArchiveDownloads {
			shouldArchive = true
		} else if logPrefix == "Twitch" && globalCfg.TwitchArchiveDownloads {
			shouldArchive = true
		}

		if shouldArchive {
			archivePath := filepath.Join(b.controller.GetStreamMonConfig().WorkingDirectory, "archive.txt")
			if err := util.AppendLineToFile(archivePath, proc.videoID); err != nil {
				proc.logger.LogError(fmt.Sprintf("Failed to archive video ID: %v", err))
			} else {
				b.archivedVidMu.Lock()
				b.archivedVideos[proc.videoID] = true
				b.archivedVidMu.Unlock()
			}
		}
	}

	proc.logger.Close()
}

// checkAllChannels concurrently checks all configured channels.
func (b *BaseMonitor) checkAllChannels() int {
	channels := b.controller.GetChannels()

	var wg sync.WaitGroup
	var errorCount atomic.Int32

	// Hoshinova uses buffer_unordered(4). We match this concurrency limit.
	// This creates a "burst" pattern (safe) rather than a "sustained drizzle" (bot-like).
	concurrencyLimit := 4
	sem := make(chan struct{}, concurrencyLimit)

	for _, ch := range channels {
		wg.Add(1)

		// Acquire semaphore slot (blocking if full)
		sem <- struct{}{}

		go func(c config.Channel) {
			// Ensure we release the slot when this goroutine finishes
			defer func() { <-sem }()
			if err := b.checkChannel(c, &wg); err != nil {
				errorCount.Add(1)
			}
		}(ch)
	}
	wg.Wait()
	return int(errorCount.Load())
}

// checkChannel is the core logic for checking a single channel's status.
func (b *BaseMonitor) checkChannel(ch config.Channel, wg *sync.WaitGroup) error {
	defer wg.Done()

	globalCfg := b.controller.GetGlobalConfig()
	logColor := b.controller.GetLogColor()
	logPrefix := b.controller.GetLogPrefix()

	newStatus, err := b.controller.CheckChannelStatus(ch, b.httpClient)
	if err != nil {
		fmt.Printf("%s [%s%s%s] Error checking %s: %v\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, err)
		return err
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
			// Check if the downloader is in a waiting state (e.g. twitch-dlp retrying after stream end)
			if proc.isWaiting != nil && proc.isWaiting.Load() {
				util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("API reports %s as offline and downloader is waiting. Terminating downloader.", ch.Name))
				if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
					proc.cmd.Process.Kill()
				}
				// Fall through to update status to offline; waitForDownload will handle cleanup
			} else {
				util.DebugLog(globalCfg, logPrefix, fmt.Sprintf("API reports %s as offline, but download is active for same stream ID (%s). Ignoring.", ch.Name, proc.videoID))
				return nil // Ignore this offline signal.
			}
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
			return nil
		}

		if !wasTracked || !previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s %sis now LIVE%s: %s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, ch.Name, util.ColorGreen, util.ColorReset, newStatus.Title)
		}
		b.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline (genuine case, safety net already passed)
		if wasTracked && previousStatus.IsLive {
			fmt.Printf("%s [%s%s%s] %s%s has gone OFFLINE%s\n", util.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, util.ColorReset, util.ColorRed, ch.Name, util.ColorReset)
		}
		b.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
	return nil
}
