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

// TwitchMonitor holds the state and logic for monitoring Twitch.
type TwitchMonitor struct {
	cfg             *config.TwitchConfig
	globalCfg       *config.GlobalConfig
	httpClient      *http.Client
	statusMutex     sync.RWMutex
	downloadMutex   sync.Mutex
	liveStatus      map[string]LiveInfo         // map[channelID]LiveInfo
	activeDownloads map[string]*downloadProcess // map[channelID]*downloadProcess
}

// NewTwitchMonitor creates a new Twitch monitor instance.
func NewTwitchMonitor(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) *TwitchMonitor {
	// Create a persistent HTTP client for reuse
	// Timeout is set higher to account for API latency and concurrent request queuing
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	return &TwitchMonitor{
		cfg:             cfg,
		globalCfg:       globalCfg,
		httpClient:      httpClient,
		liveStatus:      make(map[string]LiveInfo),
		activeDownloads: make(map[string]*downloadProcess),
	}
}

// Run starts the monitoring loops.
func (m *TwitchMonitor) Run() {
	fmt.Printf("%s [%sTwitch%s] Monitor started for %d channels.\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, len(m.cfg.Channels))
	fmt.Printf("%s [%sTwitch%s] Working Directory: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, m.cfg.StreamMon.WorkingDirectory)

	// Create working directory if it doesn't exist
	if _, err := os.Stat(m.cfg.StreamMon.WorkingDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(m.cfg.StreamMon.WorkingDirectory, 0755)
		if err != nil {
			fmt.Printf("%s [%sTwitch%s] Error creating working directory: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, err)
			return
		}
		fmt.Printf("%s [%sTwitch%s] Created working directory: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, m.cfg.StreamMon.WorkingDirectory)
	}
	// Start the download manager in the background
	go m.manageDownloads()

	// Configure the main check ticker
	checkInterval, err := time.ParseDuration(m.cfg.Scraper.PollInterval)
	if err != nil {
		fmt.Printf("%s [%sTwitch%s] Invalid poll_interval '%s', defaulting to 60s. Error: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, m.cfg.Scraper.PollInterval, err)
		checkInterval = 60 * time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Run initial check immediately
	m.checkAllChannels()

	for range ticker.C {
		m.checkAllChannels() // Then check on every tick
	}
}

// manageDownloads is a loop that periodically checks for live channels that need downloading.
func (m *TwitchMonitor) manageDownloads() {
	managerInterval := 5 * time.Second
	for {
		time.Sleep(managerInterval)

		m.statusMutex.RLock()
		// Create a copy of live channels to avoid holding the lock for too long
		liveChs := make(map[string]LiveInfo)
		for id, s := range m.liveStatus {
			if s.IsLive {
				liveChs[id] = s
			}
		}
		m.statusMutex.RUnlock()

		// Iterate in config order for priority
		for _, ch := range m.cfg.Channels {
			status, isLive := liveChs[ch.ID]
			if !isLive {
				continue
			}
			// Try to start a download. The function will handle all checks.
			m.tryStartDownload(ch, status)
		}
	}
}

// tryStartDownload checks all conditions and launches a download if appropriate.
func (m *TwitchMonitor) tryStartDownload(ch config.Channel, status LiveInfo) {
	m.downloadMutex.Lock()
	defer m.downloadMutex.Unlock()

	// 1. Check concurrency
	if len(m.activeDownloads) >= m.globalCfg.MaxConcurrentDownloads {
		return // At capacity
	}

	// 2. Check if already downloading
	if _, exists := m.activeDownloads[ch.ID]; exists {
		return
	}

	// 3. Check for lock file (in case of restart)
	lockPath := util.GetLockfilePath(m.cfg.StreamMon.WorkingDirectory, ch.Name, status.VideoID)
	if util.HasLock(lockPath) {
		return
	}

	// All clear, launch it.
	m.launchDownloader(ch, status, lockPath)
}

// launchDownloader creates a lockfile and starts the twitch-dlp subprocess.
// This function must be called with the downloadMutex held.
func (m *TwitchMonitor) launchDownloader(ch config.Channel, status LiveInfo, lockPath string) {
	// Create lockfile
	if err := util.CreateLock(lockPath); err != nil {
		fmt.Printf("%s [%sTwitch%s] Error creating lockfile for %s: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
		return
	}

	// Build command
	url := "https://www.twitch.tv/" + ch.ID
	args := append(m.cfg.StreamMon.Args, url)
	npxArgs := append([]string{"-y", "twitch-dlp"}, args...)
	cmd := exec.Command("npx", npxArgs...)

	// Create channel specific directory
	channelDir := filepath.Join(m.cfg.StreamMon.WorkingDirectory, util.SanitizeFolderName(ch.Name))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		fmt.Printf("%s [%sTwitch%s] Error creating directory for %s: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath)
		return
	}
	cmd.Dir = channelDir

	// Setup logging if enabled
	var logFile *os.File
	if m.globalCfg.SaveDownloadLogs {
		// Use the stream's creation date for a consistent log filename
		logName := fmt.Sprintf("%s_[%s].log", status.CreatedAt.UTC().Format("2006-01-02"), status.VideoID)
		f, err := os.Create(filepath.Join(channelDir, logName))
		if err == nil {
			logFile = f
			cmd.Stdout = f
			cmd.Stderr = f
		} else {
			fmt.Printf("%s [%sTwitch%s] Failed to create log file: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, err)
		}
	}

	// Start command
	if err := cmd.Start(); err != nil {
		fmt.Printf("%s [%sTwitch%s] Error starting download for %s: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
		util.DeleteLock(lockPath) // Clean up lock on failure
		if logFile != nil {
			logFile.Close()
		}
		return
	}

	fmt.Printf("%s [%sTwitch%s] %sStarted download for %s%s: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, util.ColorGreen, ch.Name, util.ColorReset, status.Title)

	// Store process info
	proc := &downloadProcess{
		cmd:      cmd,
		videoID:  status.VideoID,
		lockPath: lockPath,
		logFile:  logFile,
	}
	m.activeDownloads[ch.ID] = proc

	// Start a goroutine to wait for it to finish and clean up
	go m.waitForDownload(ch, proc)
}

// waitForDownload blocks until a download process finishes, then cleans up.
func (m *TwitchMonitor) waitForDownload(ch config.Channel, proc *downloadProcess) {
	err := proc.cmd.Wait() // This blocks until the process exits

	// Now clean up
	m.downloadMutex.Lock()
	delete(m.activeDownloads, ch.ID)
	m.downloadMutex.Unlock()

	util.DeleteLock(proc.lockPath)

	if proc.logFile != nil {
		proc.logFile.Close()
	}

	if err != nil {
		fmt.Printf("%s [%sTwitch%s] Download for %s finished with error: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
	} else {
		fmt.Printf("%s [%sTwitch%s] Download for %s finished successfully.\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name)
	}
}

// checkAllChannels concurrently checks all configured Twitch channels.
func (m *TwitchMonitor) checkAllChannels() {
	util.DebugLog(m.globalCfg, "Twitch", fmt.Sprintf("Checking live status for %d channels...", len(m.cfg.Channels)))

	var wg sync.WaitGroup
	for _, ch := range m.cfg.Channels {
		wg.Add(1)
		go m.checkChannel(ch, &wg)
	}
	wg.Wait()
}

// checkChannel is the core logic for checking a single channel's status.
func (m *TwitchMonitor) checkChannel(ch config.Channel, wg *sync.WaitGroup) {
	defer wg.Done()

	newStatus, err := CheckLiveGQL(m.httpClient, ch.ID, m.globalCfg)
	if err != nil {
		fmt.Printf("%s [%sTwitch%s] Error checking %s: %v\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, err)
		return
	}

	// --- SAFETY NET LOGIC (pre-lock check) ---
	// If API reports offline, we do a quick check to see if it's a temporary "snap"
	// before acquiring the main lock and changing the global state.
	if !newStatus.IsLive {
		m.statusMutex.RLock()
		previousStatus, wasTracked := m.liveStatus[ch.ID]
		m.statusMutex.RUnlock()

		m.downloadMutex.Lock()
		proc, isDownloading := m.activeDownloads[ch.ID]
		m.downloadMutex.Unlock()

		// If we thought it was live, a download is running, and the broadcast ID matches the one we're downloading...
		if wasTracked && previousStatus.IsLive && isDownloading && proc.videoID == newStatus.LastBroadcastID {
			// ...then it's likely a temporary API hiccup. Don't change the status.
			util.DebugLog(m.globalCfg, "Twitch", fmt.Sprintf("API reports %s as offline, but download is active for same stream ID (%s). Ignoring.", ch.Name, proc.videoID))
			return // Ignore this offline signal.
		}
	}
	// --- END SAFETY NET ---

	m.statusMutex.Lock()
	defer m.statusMutex.Unlock()

	previousStatus, wasTracked := m.liveStatus[ch.ID]

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
				fmt.Printf("%s [%sTwitch%s] %s is live but filtered out: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, ch.Name, newStatus.Title)
				m.liveStatus[ch.ID] = LiveInfo{IsLive: false}
			}
			return
		}

		if !wasTracked || !previousStatus.IsLive {
			fmt.Printf("%s [%sTwitch%s] %s%s is now LIVE%s: %s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, util.ColorGreen, ch.Name, util.ColorReset, newStatus.Title)
		}
		m.liveStatus[ch.ID] = newStatus
	} else {
		// Went offline (genuine case, safety net already passed)
		if wasTracked && previousStatus.IsLive {
			fmt.Printf("%s [%sTwitch%s] %s%s has gone OFFLINE%s\n", util.FormatTime(time.Now(), m.globalCfg.Timezone), util.ColorPurple, util.ColorReset, util.ColorRed, ch.Name, util.ColorReset)
		}
		m.liveStatus[ch.ID] = newStatus // Record that it's offline
	}
}

// MonitorTwitch is the public entry point that sets up and runs the monitor.
func MonitorTwitch(cfg *config.TwitchConfig, globalCfg *config.GlobalConfig) {
	monitor := NewTwitchMonitor(cfg, globalCfg)
	monitor.Run()
}
