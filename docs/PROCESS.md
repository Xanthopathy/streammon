# StreamMon - Application Process Flow

StreamMon is an automated orchestration tool that monitors YouTube and Twitch channels, detects live streams, applies configurable filters, and automatically archives them using specialized downloaders. This document outlines the complete process flow for both platforms.

## Version Context

- **v1.0.8**: Widened network-error detection, fixed a connection-monitor broadcast deadlock during longer outages, and made `[Diagnostic]` log tags blue.
- **v1.0.9**: Added config validation warnings, startup lockfile cleanup, safer YouTube fallback-state locking, fixed request-spacing math for fractional RPS values, logged return to normal poll intervals after errors clear, separated Twitch success diagnostics from YouTube merger checks, improved downloader wait/offline handling, and refactored utility packages without changing the external workflow.
- **v1.1.0**: Adds root-level archive files with legacy migration, YouTube members-only discovery through cookie-backed playlist checks, scoped cookie use for member downloads, `livestream_dl` as the default members-only downloader plus optional public-stream fallback, stalled `[wait]` retry termination, safer pending-success handling for long YouTube streams, yt-dlp residue cleanup, fuller subprocess logs, and broader package splits for config, monitoring, download lifecycle, logging, UTC offsets, and lockfiles.

---

## Application Startup

### 1. Configuration Loading

When the application starts (`main.go`):

1. **Environment Setup**
   - Set terminal title to "streammon" (lowercase)
   - Clear console and print ASCII banner

2. **Load Global Configuration** (`streammon_config.toml`)
   - Search order: executable directory, current working directory, then `configs/streammon_config.toml`
   - Timezone setting: `timezone`
   - `max_concurrent_downloads`: Maximum simultaneous download threads
   - `subprocess_progress_interval`: Throttle [download] progress lines (seconds)
   - `subprocess_wait_interval`: Throttle [wait] progress lines (seconds)
   - Platform enable flags: `enable_youtube`, `enable_twitch`
   - Debug flags: platform-level flags plus API/DLP-specific flags
   - Logging/archive/cleanup: `save_download_logs`, `youtube_archive_downloads`, `twitch_archive_downloads`, `clear_all_lockfiles`
   - If loading fails, use defaults
   - If individual keys are missing, invalid, or unknown, log `Config:` warnings and use defaults for invalid/missing values

3. **Load Platform-Specific Configurations**
   - **YouTube**: Load `streammon_config_yt.toml` if `enable_youtube` is true
   - **Twitch**: Load `streammon_config_twitch.toml` if `enable_twitch` is true
   - Search order: executable directory, current working directory, then matching file in `configs/`
   - If a config file is missing/invalid, that platform is disabled with a warning
   - Missing/invalid/unknown keys produce `Config:` warnings; invalid durations, empty working directories, empty args, invalid YouTube check methods, and invalid RPS values are replaced with defaults

4. **Validation**
   - Ensure at least one platform is enabled and configured
   - If both are disabled/failed to load, exit with fatal error

### 2. Initialization

1. **Optional Lockfile Cleanup**
   - If `clear_all_lockfiles: true`, startup removes old `.lock-*` files from enabled platform working directories
   - This prevents stale crash leftovers from blocking new downloads

2. **Create Working Directory**
   - Each monitor creates its configured `working_directory` if it doesn't exist
   - Example: `download_yt/` and `download_twitch/`

3. **Load Archive File** (Platform-Specific, if enabled)
   - If `youtube_archive_downloads: true`: Load `youtube_archive.txt` from the application root into memory (`archivedVideos` map)
   - If `twitch_archive_downloads: true`: Load `twitch_archive.txt` from the application root into memory (`archivedVideos` map)
   - Legacy `{working_directory}/archive.txt` files are migrated automatically: moved when possible, or merged and removed when both old and new files exist
   - Archive contains successfully downloaded video IDs; persists across application restarts
   - Log message shows count of archived video IDs loaded
   - Safety mechanism: Previous downloads won't be re-attempted even if app restarts

4. **Initialize Download Semaphore**
   - A global semaphore (buffered channel) is created with capacity = `max_concurrent_downloads`
   - Shared across both YouTube and Twitch monitors (limits overall concurrency)
   - Invalid values are warned about during config loading and replaced with defaults

5. **Start Global Connection Monitor** (Singleton)
   - `GetGlobalConnectionMonitor(globalCfg)` creates a single shared instance
   - Runs in a background goroutine and checks every 10 seconds
   - Network errors can trigger an immediate extra check without waiting for the timer
   - All monitors subscribe to this instance to receive connection state changes
   - Prevents duplicate logging by centralizing connection state management

6. **Start Monitors**
   - If YouTube is enabled, spawn `MonitorYouTube()` in a goroutine
   - If Twitch is enabled, spawn `MonitorTwitch()` in a goroutine
   - Each monitor's `Run()` method starts:
     - Subscribes to the global connection monitor
     - **Download Manager**: `manageDownloads()` loop (every 5 seconds)
   - Main thread waits for all monitors to complete (via `sync.WaitGroup`)

---

## YouTube Monitoring Process

### 1. Monitor Initialization

The YouTube monitor (`youtube.go` → `base_monitor.go`):

1. Creates a `YTMonitor` instance wrapping a `BaseMonitor`
2. Retrieves YouTube-specific configuration:
   - Poll interval from `streammon_config_yt.toml`
   - List of channels to monitor
   - yt-dlp arguments and working directory
   - `livestream_dl` arguments for the default members-only downloader path and the optional public-stream fallback path
   - Cookie/member-check settings: `cookies_file`, `member_check_all`, per-channel `member_check`, `member_downloader`, `member_check_args`
   - `download_wait_retries` threshold for ending stalled wait loops
3. Prints startup log with channel count and working directory

### 2. Continuous Polling Loop

**Frequency**: Every `poll_interval` (e.g., 60 seconds) in `streammon_config_yt.toml`, with jitter and intelligent backoff

1. **Connection Check Gate**
   - Before any work, acquire `pauseCond` lock and wait if `!connMonitor.IsConnected()`
   - If internet is down, main loop blocks until connection is restored
   - Global connection monitor will signal via `pauseCond.Broadcast()` when restored

2. **Request Rate Limiting & Spacing**
   - Calculates dynamic request spacing from two constraints:
     - **Freshness target**: `poll_interval / channelCount` (spread across polling cycle)
     - **Safety limit**: `1.0 / max_requests_per_second` (respect API rate limits)
   - Uses float math for RPS spacing, so fractional values such as `1.5` are handled consistently
   - Uses more conservative (larger) spacing from these two
   - Example: 10 channels @ 60s poll interval → ideal spacing 6s
   - Example: max_requests_per_second=0.5 (1 req every 2s) → force 2s spacing
   - Result: If safety limit is more conservative, logs warning and uses that instead
   - Applies **jittered delays** (±25% variance) to prevent bot-like perfect timing patterns

3. **Error Backoff**
   - Track `consecutiveErrors` counter (only counts non-network errors)
   - **Network Error Filtering**: DNS failures, TCP dial errors, read/write socket errors, timeouts, canceled/deadline contexts, and unreachable/down host or network errors are wrapped in `NetworkError` type
     - These are NOT counted toward backoff timers (they don't increment errorCount)
     - Why? Connection monitor already pauses operations when offline anyway
     - Network errors still trigger immediate connection checks via `TriggerImmediateCheck()`
   - If non-network errors occur on a poll:
     - Add backoff: `1 minute × consecutiveErrors` (capped at 10 minutes max)
     - Log: "Detected {errorCount} errors during poll. Staggering next poll by +{backoff}"
     - Example: First error round → +1m backoff; Second → +2m; etc.
     - Reset counter to 0 on successful poll with no errors and log that polling returned to the normal interval
   - Purpose:
     - Prevent hammer-like polling during actual API outages (non-network errors)
     - Allow graceful pause during connectivity loss without artificial backoff penalty
     - Distinguish between "internet is down" (monitored separately) vs "API is broken" (needs backoff)

4. **Initial Check**: Immediately upon monitor startup, then recurring every polling interval

#### Channel Status Check (`checkAllChannels()` → `checkChannel()`)

For each configured YouTube channel:

1. **Fetch Live Status**
   - Calls `CheckChannelStatus()` for YouTube
   - Uses the configured `check_method` first: `rss` or `live`
   - RSS mode constructs `https://www.youtube.com/feeds/videos.xml?channel_id={ID}`, parses the latest entry, extracts video ID/title/timestamp, and ignores entries older than `ignore_older_than`
   - Live-page mode checks the channel live page directly
   - If the first method fails with a non-network error, the other method is tried as fallback
   - If fallback succeeds, that fallback method is kept for the channel until `fallback_duration` expires
   - Fallback state is mutex-protected so concurrent channel checks cannot race the fallback map
   - If regular checks report offline or all regular methods fail, and `member_check_all` or that channel's `member_check` is enabled, run a members-only playlist check through `yt-dlp`
   - Member checks use `cookies_file` plus `member_check_args`, parse the returned playlist JSON, and mark matching live entries as `Source = "members"`

2. **Handle Status Response**
   - **Error**: Log error, skip channel, continue
   - **Offline**: Update internal status map
   - **Online**: Proceed to next checks

3. **Filter Matching** (if live)
   - If no filters defined: match all streams
   - If filters defined: apply each regex pattern against video title
   - If match found: proceed to download
   - If no match: mark as live but ignore (log in debug mode)

4. **State Tracking**
   - Store live status in internal map: `liveStatus[channelID] = LiveInfo`
   - Log transitions: "is now LIVE" or "has gone OFFLINE"

### 3. Download Manager Loop

**Frequency**: Every 5 seconds (independent of polling)

1. **Scan Live Channels**
   - Iterate through all channels with `IsLive: true`
   - For each live channel, call `tryStartDownload()`

2. **Pre-Flight Checks** (`tryStartDownload()`)

   a. **Archive Check**
   - Check if this video ID exists in `archivedVideos` map (loaded from the platform archive at startup)
   - If yes: Video was previously downloaded successfully; skip with log message "already downloaded in archive"
   - Purpose: Prevents re-downloading the same video across app restarts

   b. **Session Cache Check**
   - Check if this video was already downloaded in current app instance
   - Maintains in-memory map: `downloadedVideos[channelID][videoID]`
   - If yes: Video is queued or already downloaded in THIS session; skip
   - Log behavior (controlled by tracking maps):
     - First time skip → log message with "DownloadedVidsLogged" tracking
     - Subsequent skips in same session → don't spam logs (use `downloadedVidsLogged` map to track)
   - Purpose: Prevents redundant downloads of same video within same session while keeping logs clean

   c. **Global Capacity Check**
   - Attempt to acquire a slot from the download semaphore (`downloadSlots`)
   - If all slots are in use, skip this channel (retry next manager cycle in 5 seconds)
   - If acquired and verbose debug enabled, log: "Acquired download slot. Slots used: X/Y"
   - The slot is acquired before pre-flight checks and is released by a deferred cleanup path unless launch succeeds

   d. **Local Download Check**
   - Check if a download for this channel is already running (in `activeDownloads` map)
   - If yes, skip

   e. **Lockfile Check**
   - Check if lockfile exists: `.lock-{sanitized_channel_name}-{videoID}`
   - If yes, release semaphore slot and skip
   - Purpose: Handles app crashes, concurrent instances, or partial downloads

3. **Launch Download** (`launchDownloader()`)

   a. **Create Lockfile**
   - Create `.lock-{sanitized_channel_name}-{videoID}` file in working directory
   - If creation fails, release slot, log error, and return
   - Log event: "LOCK: Created: {lockPath}"

   b. **Create Channel Directory**
   - Create `{working_directory}/{sanitized_channel_name}/` if needed (lowercase, spaces→underscores)
   - Download will execute in this directory

   c. **Setup Logging** (if `save_download_logs: true`)
   - Create single log file: `{channel_dir}/{sanitized_channel_name}-{videoID}.log`
   - Capture subprocess stdout/stderr via pipe redirection
   - Subprocess output is written to log file with throttling applied:
     - `[download]` progress lines throttled by `subprocess_progress_interval` (30s default)
     - `[wait]` / `[retry-streams]` lines throttled by `subprocess_wait_interval` (600s default)
     - All other lines logged immediately
   - Terminal visibility controlled by platform-specific DLP debug flags (independent of file logging)
   - Initialize logger instance with metadata (channel ID, name, video ID, creation timestamp, command being executed)

   d. **Output Callback Setup**
   - Register callback function to detect subprocess state:
     - If line contains `[retry-streams]`: Set `isWaiting = true` (stream not yet available for download)
     - If line contains `frame=` or `[download]`: Set `isWaiting = false` (active downloading in progress)
     - If line contains `[Merger]` or `Merging formats`: Set YouTube merger detection
     - If line contains Twitch/ffmpeg completion markers such as `[stats] Fragments`, final `frame=...Lsize=`, or muxing overhead: Set generic download completion detection
   - Purpose: Detect when process is just waiting for live stream vs actively downloading
   - Used for intelligent progress reporting and timeout handling

   e. **Build Downloader Command**
   - Regular YouTube streams use `yt-dlp` with arguments from config
   - Command: `yt-dlp [args from config] https://www.youtube.com/watch?v={videoID}`
   - Members-only streams use `member_downloader`, which defaults to `livestream_dl`; `yt-dlp` remains configurable but can stall with member cookies
   - Cookies are added automatically only for members-only checks/downloads when the configured args do not already include `--cookies` or `--cookies-from-browser`
   - Regular stream downloads do not auto-add cookies; cookies for public streams remain an explicit user config choice
   - Working directory: channel-specific directory
   - Env variables: Set `FORCE_COLOR=1` and `TERM=xterm-256color` to enable color output

   f. **Setup Subprocess Piping**
   - If logging enabled OR dlpDebug enabled, pipe stdout and stderr
   - Spawn goroutines `ReadPipeAndLog()` to capture and log output in real-time
   - Each piped line is throttled according to its type ([download], [wait], etc.)

   g. **Start Process**
   - Execute command via `cmd.Start()` (non-blocking)
   - If start fails, delete lockfile, release slot, log error, and return
   - Log: "[YT] Started download for {channel}: {title}"

   h. **Track Process**
   - Store process info in `activeDownloads[channelID]` with process handle
   - Spawn goroutine `waitForDownload()` to monitor completion asynchronously

### 4. Download Completion (YouTube)

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`
   - Detects both successful completion and crashes

2. **Reset Terminal Title**
   - Reset title to "streammon" when process completes (prevents subprocesses from changing it)

3. **Release Resources** (immediately upon exit)
   - Wait 5 seconds after `cmd.Wait()` so downloader/ffmpeg cleanup and file flushes can settle
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile: "LOCK: Deleted: {lockPath}"
   - Close log file if exists

4. **Determine Success** (Two-Condition Verification)
   - **Extract exit code** from subprocess (may be non-zero even on success)
   - **Check for completion markers** in output:
     - Detect `[Merger]` or `Merging formats` in subprocess output (indicates successful format merge)
     - `mergerDetected = true` if marker found
   - **Verify output file exists** in working directory:
     - Look for `.mp4`, `.mkv`, or `.webm` files containing the video ID
     - `outputFileExists = true` if file found
   - **Success condition**: `mergerDetected && outputFileExists`
     - This handles the yt-dlp quirk: exit code 1 with warnings but output file IS created
     - Previous implementation trusted exit codes → false positives when warnings occurred
     - New implementation verifies actual completion + file creation instead
   - If yt-dlp fails and `livestream_dl.enabled` is true, the same download process tries a one-time `livestream_dl` fallback before releasing the slot or deleting the lockfile
   - `livestream_dl` fallback success requires exit code 0 and a matching media file
   - **Forced termination**: If download was stopped by monitor (stream went offline), treat as success (meaningful data captured)

5. **Log Completion**
   - **Diagnostic info** (always logged): `[Diagnostic] yt-dlp exit code: {code} | merger_detected: {bool} | file_exists: {bool}`
     - Provides visibility for debugging without affecting success logic
   - **If forced termination**: Log "[YT] Download for {channel} stopped by monitor (stream offline)."
   - **If success** (both conditions met): Log "[YT] Download for {channel} finished successfully."
   - **If `livestream_dl` fallback succeeds**: Log that the download finished successfully with the fallback downloader
   - **If failure** (one or both conditions missing):
     - Log "[YT] Download for {channel} finished with error: {error} (exit_code={code}, reasons={list})"
     - Reasons list includes: "no_merger_detected" and/or "output_file_not_found"

6. **Persist to Archive** (if `youtube_archive_downloads: true` and download succeeded)
   - Append video ID to root-level `youtube_archive.txt`
   - Format: One video ID per line (same format as yt-dlp's archive file)
   - Purpose: Ensure video won't be re-downloaded even after app restart
   - For normal YouTube completion, archiving waits until the next poll confirms the stream is no longer live; if the same video ID is still live, another download attempt is allowed instead of prematurely archiving a still-running stream

7. **Update Session Cache** (on success)
   - Add video ID to session cache: `downloadedVideos[channelID][videoID] = true`
   - Prevents re-downloading the same video in subsequent polling cycles
   - Cache is temporary (cleared on app restart)

### 5. Safety Net Logic

**After API reports stream as offline:**

- If a download is **actively running** for the same stream ID, **ignore** the offline signal
- If the subprocess is only waiting/retrying (`[retry-streams]`), mark it as forced termination and interrupt the process
- If `[wait]` retry lines reach `download_wait_retries`, stop the stalled downloader wait loop; `0` disables this guard
- Forced termination is treated as successful capture because meaningful data may already be on disk
- This prevents premature abortion during API lag while still breaking out of endless retry/wait states after a stream really ends

---

## Twitch Monitoring Process

### 1. Monitor Initialization

The Twitch monitor (`twitch.go` → `base_monitor.go`):

1. Creates a `TwitchMonitor` instance wrapping a `BaseMonitor`
2. Retrieves Twitch-specific configuration:
   - Poll interval from `streammon_config_twitch.toml`
   - List of channels to monitor
   - twitch-dlp arguments and working directory
3. Prints startup log with channel count and working directory

### 2. Continuous Polling Loop

**Frequency**: Every `poll_interval` (e.g., 30 seconds) in `streammon_config_twitch.toml`, with jitter and intelligent backoff

1. **Connection Check Gate**
   - Before any work, acquire `pauseCond` lock and wait if `!connMonitor.IsConnected()`
   - If internet is down, main loop blocks until connection is restored
   - Global connection monitor will signal via `pauseCond.Broadcast()` when restored

2. **Request Rate Limiting & Spacing**
   - Calculates dynamic request spacing from two constraints:
     - **Freshness target**: `poll_interval / channelCount` (spread across polling cycle)
     - **Safety limit**: `1.0 / max_requests_per_second` (respect GraphQL rate limits)
   - Uses float math for RPS spacing, so fractional values such as `1.5` are handled consistently
   - Uses more conservative (larger) spacing from these two
   - Applies **jittered delays** (±25% variance) to prevent bot-like perfect timing patterns
   - If safety limit is more conservative, logs warning

3. **Error Backoff**
   - Track `consecutiveErrors` counter (only counts non-network errors)
   - **Network Error Filtering**: DNS failures, TCP dial errors, read/write socket errors, timeouts, canceled/deadline contexts, and unreachable/down host or network errors are wrapped in `NetworkError` type
     - These are NOT counted toward backoff timers (they don't increment errorCount)
     - Why? Connection monitor already pauses operations when offline anyway
     - Network errors still trigger immediate connection checks via `TriggerImmediateCheck()`
   - If non-network errors occur on a poll:
     - Add backoff: `1 minute × consecutiveErrors` (capped at 10 minutes max)
     - Example: First error round → +1m backoff; Second → +2m; etc.
     - Reset counter to 0 on successful poll with no errors and log that polling returned to the normal interval
   - Purpose:
     - Prevent hammer-like polling during actual API outages (non-network errors)
     - Allow graceful pause during connectivity loss without artificial backoff penalty
     - Distinguish between "internet is down" (monitored separately) vs "API is broken" (needs backoff)

4. **Initial Check**: Immediately upon monitor startup, then recurring every polling interval

#### Channel Status Check (`checkAllChannels()` → `checkChannel()`)

For each configured Twitch channel:

1. **Fetch Live Status**
   - Calls `CheckChannelStatus()` → `CheckLiveGQL()`
   - Executes GraphQL query against Twitch API
   - Retrieves:
     - Stream live status (`isLiveBroadcast`)
     - Video/broadcast ID
     - Stream title
     - Stream creation timestamp
   - Returns `LiveInfo` with `IsLive` flag and metadata

2. **Handle Status Response**
   - **Error**: Log error, skip channel, continue
   - **Offline**: Update internal status map
   - **Online**: Proceed to next checks

3. **Filter Matching** (if live)
   - If no filters defined: match all streams
   - If filters defined: apply each regex pattern against stream title
   - If match found: proceed to download
   - If no match: mark as live but ignore (log in debug mode)

4. **State Tracking**
   - Store live status in internal map: `liveStatus[channelID] = LiveInfo`
   - Log transitions: "is now LIVE" or "has gone OFFLINE"

### 3. Download Manager Loop

**Frequency**: Every 5 seconds (independent of polling)

1. **Scan Live Channels**
   - Iterate through all channels with `IsLive: true`
   - For each live channel, call `tryStartDownload()`

2. **Pre-Flight Checks** (`tryStartDownload()`)

   a. **Archive Check**
   - Check if this broadcast/stream ID exists in `archivedVideos` map (loaded from the platform archive at startup)
   - If yes: Stream was previously downloaded successfully; skip with log message
   - Purpose: Prevents re-downloading the same stream across app restarts

   b. **Session Cache Check**
   - Check if this broadcast was already downloaded in current app instance
   - Maintains in-memory map: `downloadedVideos[channelID][broadcastID]`
   - If yes: Stream is queued or already downloaded in THIS session; skip
   - Log tracking prevents log spam (uses `downloadedVidsLogged` map)
   - Purpose: Prevents redundant downloads within same session while keeping logs clean

   c. **Global Capacity Check**
   - Attempt to acquire a slot from the download semaphore
   - If all slots are in use, skip this channel (retry next manager cycle in 5 seconds)
   - If acquired and verbose debug enabled, log: "Acquired download slot. Slots used: X/Y"

   d. **Local Download Check**
   - Check if a download for this channel is already running (in `activeDownloads` map)
   - If yes, skip

   e. **Lockfile Check**
   - Check if lockfile exists: `.lock-{sanitized_channel_name}-{broadcastID}`
   - If yes, release semaphore slot and skip

3. **Launch Download** (`launchDownloader()`)

   a. **Create Lockfile**
   - Create `.lock-{sanitized_channel_name}-{broadcastID}` file in working directory
   - If creation fails, release slot, log error, and return
   - Log event: "LOCK: Created: {lockPath}"

   b. **Create Channel Directory**
   - Create `{working_directory}/{sanitized_channel_name}/` if needed (lowercase, spaces→underscores)
   - Download will execute in this directory

   c. **Setup Logging** (if `save_download_logs: true`)
   - Create single log file: `{channel_dir}/{sanitized_channel_name}-{broadcastID}.log`
   - Capture subprocess stdout/stderr via pipe redirection
   - Subprocess output is written to log file with throttling applied:
     - `[download]` progress lines throttled by `subprocess_progress_interval` (30s default)
     - `[wait]` / `[retry-streams]` lines throttled by `subprocess_wait_interval` (600s default)
     - All other lines logged immediately
   - Terminal visibility controlled by platform-specific DLP debug flags (independent of file logging)
   - Initialize logger instance with metadata

   d. **Output Callback Setup**
   - Register callback function to detect subprocess state:
     - If line contains `[retry-streams]`: Set `isWaiting = true` (waiting for stream to go live)
     - If line contains `frame=` or `[download]`: Set `isWaiting = false` (actively downloading)
   - Purpose: Detect download state for intelligent progress reporting

   e. **Build Downloader Command**
   - Twitch uses `twitch-dlp` via npm/npx
   - Command: `npx -y twitch-dlp [args from config] https://www.twitch.tv/{channelLogin}`
   - Working directory: channel-specific directory
   - Env variables: Set `FORCE_COLOR=1` and `TERM=xterm-256color` to enable color output
   - Note: `-y/--yes` flag auto-updates twitch-dlp

   f. **Setup Subprocess Piping**
   - If logging enabled OR dlpDebug enabled, pipe stdout and stderr
   - Spawn goroutines `ReadPipeAndLog()` to capture and log output in real-time
   - Each piped line is throttled according to its type

   g. **Start Process**
   - Execute command via `cmd.Start()` (non-blocking)
   - If start fails, delete lockfile, release slot, log error, and return
   - Log: "[Twitch] Started download for {channel}: {title}"

   h. **Track Process**
   - Store process info in `activeDownloads[channelID]` with process handle
   - Spawn goroutine `waitForDownload()` to monitor completion asynchronously

### 4. Download Completion (Twitch)

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`
   - Detects both successful completion and crashes

2. **Reset Terminal Title**
   - Reset title to "streammon" when process completes

3. **Release Resources** (immediately upon exit)
   - Wait 5 seconds after `cmd.Wait()` so downloader/ffmpeg cleanup and file flushes can settle
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile: "LOCK: Deleted: {lockPath}"
   - Close log file if exists

4. **Determine Success and Log Completion**
   - Extract exit code from subprocess
   - Detect Twitch/ffmpeg completion markers such as `[stats] Fragments`, final `frame=...Lsize=`, or muxing overhead
   - Verify an output media file exists in the channel directory:
     - For Twitch, file matching is based on files touched by this run because twitch-dlp output IDs may differ from the live GQL stream ID
   - Success condition: `outputFileExists && (downloadCompleted || exitCode == 0)`
   - Forced termination by the monitor is treated as success
   - Diagnostic info is logged as `[Diagnostic] twitch-dlp exit code: {code} | completion_detected: {bool} | file_exists: {bool}`
   - Failure reasons include `no_completion_detected` and/or `output_file_not_found`

5. **Persist to Archive** (if `twitch_archive_downloads: true` and download succeeded)
   - Append broadcast ID to root-level `twitch_archive.txt`
   - Format: One broadcast ID per line
   - Purpose: Ensure broadcast won't be re-downloaded even after app restart

6. **Update Session Cache** (on success)
   - Add broadcast ID to session cache: `downloadedVideos[channelID][broadcastID] = true`
   - Prevents re-downloading the same broadcast in subsequent polling cycles
   - Cache is temporary (cleared on app restart)

### 5. Safety Net Logic

**After API reports stream as offline:**

- If a download is **actively running** for the same broadcast ID, **ignore** the offline signal
- If the subprocess is only waiting/retrying (`[retry-streams]`), mark it as forced termination and interrupt the process
- Forced termination is treated as successful capture because meaningful data may already be on disk
- This prevents premature abortion during API lag while still breaking out of endless retry/wait states after a stream really ends

---

## Connection Monitoring

### Global Connection Monitor (Singleton - `connection.go`)

Runs once as a singleton instance in a background goroutine, independent of polling/download manager. All monitors subscribe to this single instance, providing centralized pause/resume capability when internet connectivity is lost.

**Why Global?**

- **Single source of truth**: All monitors check the same connection state
- **Prevents duplicate logging**: Connection state changes logged only once, not per monitor
- **Efficient error handling**: Network errors count once toward backoff timers, not per-monitor
- **Reduced system load**: One connection checker instead of N (one per monitor)

**Hysteresis-Based Stability Detection:**

```
Global Connection Status Flow:
├─ Start: isConnected = true, lastLogged = true
├─ Loop: Every 10 seconds, or immediately when a network error triggers a check
│
├─ Attempt connection check: CheckInternetConnection()
│  └─ Rotates through 4 reliable hosts with fallback:
│     ├─ Primary: Cloudflare DNS (1.1.1.1:53)
│     ├─ Fallback 1: Google DNS (8.8.8.8:53)
│     ├─ Fallback 2: Quad9 DNS (9.9.9.9:53)
│     └─ Fallback 3: OpenDNS (208.67.222.222:53)
│     └─ Returns true if ANY host responds within 3-second timeout
│
├─ Success:
│  ├─ If already connected: reset failureCounter
│  ├─ If disconnected: increment successCounter and log "Connection check passed (N/3)..."
│  ├─ If successCounter >= 3:
│  │  ├─ Set isConnected = true
│  │  ├─ Reset failureCounter = 0
│  │  ├─ Check if lastLogged is false (prevents duplicate logs)
│  │  ├─ If yes: Set lastLogged = true, broadcast to all subscribers
│  │  ├─ Log: "Connection restored (stable). Resuming operations..."
│  └─ Else: No action (still verifying)
│
└─ Failure:
   ├─ Increment failureCounter++
   ├─ If failureCounter == 1:
   │  ├─ Log: "WARN Connection check failed. Verifying stability..."
   ├─ If failureCounter >= 3:
   │  ├─ Set isConnected = false
   │  ├─ Reset successCounter = 0
   │  ├─ Check if lastLogged is true (prevents duplicate logs)
   │  ├─ If yes: Set lastLogged = false, broadcast to all subscribers
   │  ├─ Log: "Connection lost (confirmed). Pausing monitors..."
   └─ Else: No action (still verifying)
```

**Parameters:**

- `normalInterval`: 10 seconds
- `threshold`: 3 consecutive successes OR failures needed to change state (prevents flapping)
- `checkTrigger`: buffered channel used to request an immediate check when a live API/RSS call detects a network error

**Connection Check Method (`CheckInternetConnection()`):**

- Rotates through 4 reliable public DNS servers
- Primary check: Connect to next host in rotation (round-robin)
- If primary fails, tries remaining 3 hosts as fallback
- Timeout: 3 seconds per host (fail fast on timeouts)
- Returns: `true` if ANY host responds, `false` if all fail
- Purpose: More robust than single-host check; handles regional DNS issues

**Triggered Immediate Checks:**

- When checker.go detects network errors ("no such host", "dial tcp", etc.), it calls `connMonitor.TriggerImmediateCheck()`
- Connection monitor processes this immediately without waiting for next timer interval
- Enables faster detection and response to connection issues

**Integration with Monitors:**

1. During `Run()` startup:
   - Each monitor calls `GetGlobalConnectionMonitor(globalCfg)` (returns singleton)
   - Calls `connMonitor.Subscribe(b.pauseCond)` to receive broadcasts

2. During main polling loop:
   - Before work, acquire `pauseCond` lock
   - Check `connMonitor.IsConnected()`
   - If offline, call `pauseCond.Wait()` (blocks indefinitely)

3. When connection state changes:
   - Global monitor calls `pauseCond.Broadcast()` for all subscribers
   - All monitors wake up and resume/pause accordingly

**Duplicate Logging Prevention:**

- Tracks `lastLogged` boolean to record the last logged state
- When connection state changes:
  - If `isConnected=true` AND `lastLogged=false`: Log "Connection restored", set `lastLogged=true`
  - If `isConnected=false` AND `lastLogged=true`: Log "Connection lost", set `lastLogged=false`
- This ensures each state change logs exactly once, even with multiple monitors

**Network Error Filtering (Prevents Backoff During Outages):**

The system intelligently distinguishes between network errors and actual API errors:

1. **Network Error Detection** (in `checkChannel()`)
   - Covers DNS failures, TCP dial errors, socket read/write errors, timeouts, canceled/deadline contexts, and unreachable/down host or network errors
   - These are wrapped in `NetworkError` type to mark them as connectivity issues
   - If the connection monitor still thinks the app is online, log a warning for visibility
   - Trigger immediate connection check: `connMonitor.TriggerImmediateCheck()`

2. **Error Counting** (in `checkAllChannels()`)
   - Loop through all channel check results
   - Only count errors that are NOT `NetworkError` type
   - Network errors are skipped: `if !IsNetworkError(err) { errorCount.Add(1) }`

3. **Result**
   - **Network errors don't trigger backoff timers** (no artificial delay penalty)
   - **Why?** Connection monitor already pauses the main loop when offline
   - **Real API errors still trigger backoff** (e.g., GraphQL errors, 429 rate limits, 500 server errors)
   - **Distinction**: Network layer issues → paused by connection monitor | API layer issues → backed off by error counter

**Example Timeline - Internet Outage:**

```
16:07:16 [System] Connection check failed. Verifying stability...     (1st failure)
16:07:16 [YT] ERROR: Error checking Eimi: no such host               (Network error - NOT counted)
16:07:17 [System] Connection check failed. Verifying stability...     (2nd failure)
16:07:17 [Twitch] ERROR: Error checking Komachi: dial tcp            (Network error - NOT counted)
16:07:21 [System] Connection lost (confirmed). Pausing monitors...    (3rd failure threshold reached)
         ↑↑↑ Main loop blocks here, no more API requests until restored ↑↑↑
16:07:22 [YT] [waiting - offline]                                     (Blocked, not polling)
16:33:29 [System] Connection restored (stable). Resuming operations... (3 successes)
         ↑↑↑ Main loop resumes, errorCount is still 0 (no backoff penalty) ↑↑↑
```

**Result:**

- When internet is down: No errors in logs, no API requests, graceful pause across all monitors
- When internet returns: Automatic resume after 3 consecutive successful checks; network errors can trigger immediate checks between the regular 10-second ticks
- Prevents "flapping" (rapid on/off toggling) with hysteresis counters
- No artificial backoff penalty when connection is lost
- No duplicate log messages regardless of monitor count

---

### 1. Concurrent Polling

- **YouTube** and **Twitch** monitors run in separate goroutines
- Each has an independent polling loop and download manager
- They share a **global download semaphore** to limit overall concurrency

### 2. Download Slot Management

- **Global Semaphore**: Buffered channel with capacity = `max_concurrent_downloads`
- **Acquisition**: Slot reserved when download is about to start
- **Release**: Slot released when:
  - Pre-flight checks fail (attempted to acquire but needs cleanup)
  - Download process exits (success or failure)

### 3. Deduplication (Multi-Layer)

**Three-Layer Deduplication System:**

- **Layer 1 - Archive File**: root-level `youtube_archive.txt` or `twitch_archive.txt`
  - Persistent storage of successfully downloaded video IDs
  - Loaded into `archivedVideos` map at monitor startup
  - Appended to on every successful download
  - Survives application restarts and crashes
  - Prevents re-downloading the same video across any future app instance
  - Legacy `{working_directory}/archive.txt` files are migrated into the root-level archive
  - Note: Can be manually edited / reset by deleting the platform archive file

- **Layer 2 - In-Memory Session Cache**: `downloadedVideos[channelID][videoID]`
  - Tracks downloads detected and downloaded in current app instance
  - Used to prevent re-queueing of pending downloads
  - Discarded when app exits (temporary)
  - Prevents redundant downloads of videos detected in same session

- **Layer 3 - Lockfiles**: `.lock-{sanitized_channel_name}-{videoID}` in working directory
  - Files in working directory exist while download is in-progress or crashed
  - Prevents multiple instances of app from downloading same video concurrently
  - Prevents re-launching downloads for partially-complete files
  - Lifecycle: Created before launch, deleted after process exit
  - Survives across app restarts (even if app crashes mid-download)
  - Purpose: Handle app crashes, concurrent instances, cleanup detection

**Deduplication Decision Tree:**

```
Is video in platform archive file? (Persistent)
├─ YES: SKIP ✓ (prevent re-download across restarts)
├─ NO: Continue to next check
│
Is video in session cache? (Active app)
├─ YES: SKIP ✓ (prevent redundant queue in same session)
├─ NO: Continue to next check
│
Is lockfile present?
├─ YES: SKIP ✓ (download already in progress or crashed)
├─ NO: Continue to launch
│
LAUNCH DOWNLOAD ✓
└─ On success: Add to platform archive file + session cache
└─ On crash: Lockfile remains (will be detected on next app instance)
```

**Benefits:**

- Robust across restarts, crashes, and concurrent instances
- Prevents bandwidth waste from duplicate downloads
- Prevents disk space waste from multiple copies
- Self-healing: Cleanup on next app restart if lockfile left behind

### 4. State Monitoring

**Live Status Map**: `map[channelID]LiveInfo`

- Tracks current live status for each channel
- Protected by `statusMutex` (RWMutex for concurrent reads, exclusive writes)
- Updated on every polling cycle

**Active Downloads Map**: `map[channelID]*downloadProcess`

- Tracks currently-running downloads by channel
- Protected by `downloadMutex`
- Prevents concurrent downloads for same channel

**Session Cache (Downloads)**: `map[channelID]map[videoID]bool`

- In-memory cache of successfully downloaded IDs in current session
- Protected by `downloadedVidMu` (RWMutex)
- Persists across polling cycles but cleared on app restart

**Archive Cache**: `map[videoID]bool`

- Loaded from the platform archive file at startup
- Protected by `archivedVidMu` (RWMutex)
- Used for quick lookup of previously downloaded IDs

**Logging Tracking Maps**:

- `queuedVideosLogged`: Track which queued videos have logged "already queued" message
- `downloadedVidsLogged`: Track which downloaded videos have logged "already downloaded" message
- Purpose: Log once per video ID per app instance (prevent log spam)
- Protected by their respective mutexes

### 5. Process Isolation

- Each download runs in a separate subprocess (via `cmd.Start()`)
- Crash of one downloader doesn't crash monitor
- Downloader process exits release their semaphore slots and delete lockfiles in `waitForDownload()`
- Full app crashes can leave lockfiles behind; `clear_all_lockfiles` can remove those at the next startup
- Each subprocess gets its own logger instance for isolated output capture

### 6. Logging

#### Log File

**Single `.log` file per download** (if `save_download_logs: true`):

- Location: `{channel_dir}/{sanitized_channel_name}-{videoID}.log`
- Contains subprocess command executed (logged on startup)
- All subprocess output from twitch-dlp/yt-dlp (every line captured)
- Each line tagged with source: `[yt-dlp]` or `[twitch-dlp]`
- Throttling applied per line type (separate throttling counters):
  - `[download]` progress lines: Throttled by `subprocess_progress_interval` (30s default)
  - `[wait]`/`[retry-streams]` lines: Throttled by `subprocess_wait_interval` (600s default)
  - All other lines: Logged immediately
- Log file always receives all output (independent of debug flags)
- Created via `NewLoggerForDownload()` with full metadata

#### Terminal Output

**Always shown:**

- Monitor startup/shutdown messages
- Channel status transitions ("is now LIVE", "has gone OFFLINE")
- Download start/completion with title and status
- Important errors and warnings
- Connection state changes ("Connection lost", "Connection restored")
- Session cache hits ("already downloaded in this session")

**Conditional (debug flags control visibility):**

- If `youtube_verbose_debug: true`: YouTube monitor debug output
- If `twitch_verbose_debug: true`: Twitch monitor debug output
- If `youtube_api_verbose_debug: true`: YouTube RSS API calls and responses (very verbose)
- If `twitch_api_verbose_debug: true`: Twitch GraphQL API calls and responses (very verbose)
- If `youtube_dlp_verbose_debug: true`: Raw yt-dlp subprocess output in terminal (with throttling on [download]/[wait])
- If `twitch_dlp_verbose_debug: true`: Raw twitch-dlp subprocess output in terminal (with throttling on [download]/[wait])
- **Note:** DLP verbose flags control **terminal** printing only; log files always receive all subprocess output

#### Terminal Colors

- **Colored Terminal Output**: Different colors for YouTube (red), Twitch (purple), System (cyan), Info (blue)
- **Timestamps**: All terminal output includes timestamp with configurable timezone (IANA timezone names like "Asia/Tokyo" or UTC offsets like "UTC+7")
- **Subprocess lines** tagged with `[yt-dlp]` or `[twitch-dlp]` and subject to throttling
- **Debug/event lines** use colored tags such as `[YouTubeAPI]`, `[TwitchAPI]`, `[LOCK]`, `[WARN]`, and `[Diagnostic]`

#### Output Callback System

Each download subprocess has a callback function that monitors output:

```go
outputCallback := func(line string) {
    if strings.Contains(line, "[retry-streams]") {
        isWaiting.Store(true)  // Stream not yet live
    } else if strings.Contains(line, "frame=") || strings.Contains(line, "[download]") {
        isWaiting.Store(false) // Active downloading
    }
    if strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats") {
        mergerDetected.Store(true)
    }
    if strings.Contains(line, "[stats] Fragments") ||
        (strings.Contains(line, "frame=") && strings.Contains(line, "Lsize=")) ||
        (strings.Contains(line, "[out#") && strings.Contains(line, "muxing overhead:")) {
        downloadCompleted.Store(true)
    }
}
```

- Purpose: Detect if subprocess is waiting vs actively downloading
- Used for intelligent timeout, forced termination, and downloader-specific completion checks
- Runs in real-time as output is piped from subprocess

---

## Configuration Files

### Global Config (`streammon_config.toml`)

```toml
timezone = "UTC"
max_concurrent_downloads = 10
enable_youtube = true
enable_twitch = true
save_download_logs = true
subprocess_progress_interval = 30     # Throttle [download] lines (seconds)
subprocess_wait_interval = 600        # Throttle [wait] lines (seconds)
youtube_archive_downloads = true
twitch_archive_downloads = true
clear_all_lockfiles = true
youtube_verbose_debug = true
twitch_verbose_debug = true
youtube_api_verbose_debug = true
twitch_api_verbose_debug = false
youtube_dlp_verbose_debug = true
twitch_dlp_verbose_debug = true
```

### YouTube Config (`streammon_config_yt.toml`)

```toml
[streammon]
working_directory = "download_yt"
args = ["--wait-for-video", "60", "--live-from-start", ...]

[livestream_dl]
# Used by default for members-only downloads; `enabled` controls only the public-stream fallback path.
enabled = false
args = ["--resolution", "best", "--threads", "4", "--segment-retries", "10", ...]

[scraper]
poll_interval = "120s"
ignore_older_than = "24h"
check_method = "rss"
fallback_duration = "15m"
max_requests_per_second = 2
cookies_file = "youtube_cookies.txt"
member_check_all = false
member_downloader = "livestream_dl"
download_wait_retries = 3
member_check_args = ["--flat-playlist", "--playlist-items", "1:3", "--dump-single-json", "--no-warnings"]

[[channel]]
id = "UC..."
name = "Channel Name"
filters = ["(?i).*karaoke.*"]
member_check = false
```

### Twitch Config (`streammon_config_twitch.toml`)

```toml
[streammon]
working_directory = "download_twitch"
args = ["--live-from-start", "--retry-streams", "60", ...]

[scraper]
poll_interval = "120s"
max_requests_per_second = 2

[[channel]]
id = "channel_login"
name = "Channel Display Name"
filters = ["(?i).*english.*"]
```

---

## Filter Behavior

- **No Filters**: Accept all live streams
- **With Filters**: Accept only streams matching at least one regex pattern
- **Regex Matching**: Case-insensitive by default (use `(?i)` prefix)
- **Logging**: If live but filtered out, log "[Platform] {channel} is live but filtered out: {title}"

---

## Error Handling

### Configuration Errors

- Missing global config: Use defaults with a warning
- Missing platform config: Disable that platform with a warning
- Missing keys: Log `Config:` warning and use the default value already loaded into the config struct
- Unknown keys: Log `Config:` warning and ignore the value
- Invalid poll intervals/durations: Log `Config:` warning and use defaults
- Invalid lockfile paths: Log error, skip download, release slot
- Invalid timezone: Log `Config:` warning and fall back to UTC

### API Errors (YouTube RSS / Twitch GraphQL)

- HTTP request failures: Log error, increment consecutive error counter, continue monitoring
- Parsing errors (XML, JSON): Log error, skip channel, continue
- Rate limiting (HTTP 429): Caught as generic error, contributes to backoff logic
- Fallback activation (YouTube): If primary method fails N times, switch to secondary method

### Connection Errors

- Network timeouts: Caught during API calls
- DNS resolution failures: Caught during connection checks
- TCP connection failures: Trigger hysteresis-based pause mechanism
- Temporary vs persistent: Hysteresis (3-threshold) prevents false pauses

### Download Errors

- Command start failure: Log error, delete lockfile, release slot, close logger
- Process crash during download: Automatically cleaned up via `waitForDownload()` goroutine
- Process exit with non-zero code: Still release resources; archive only if downloader-specific success checks pass or the monitor forced termination
- Log file creation failure: Log warning, continue download without log output

### Resource Cleanup on Errors

All errors in `launchDownloader()` or `waitForDownload()` are guaranteed to:

1. Release semaphore slot (`<-downloadSlots`)
2. Delete lockfile (if created)
3. Close logger (if created)
4. Update active download tracking
5. Log the error for visibility

This prevents resource leaks while the app remains running. If the whole app is killed, startup lockfile cleanup can remove stale `.lock-*` files on the next run.

### Backoff & Retry Logic

- **Poll Errors**: Add 1-minute backoff per consecutive error (capped at 10 minutes)
- **RPS Throttling**: Warn once if poll_interval is too short for channel count and max RPS
- **Request Spacing**: Automatically increased if RPS safety limit is more conservative than freshness target
- **Recovery Logging**: When errors clear, log that polling is returning to the normal poll interval
- **Connection Recovery**: Resume automatically after 3 consecutive successful connection checks; immediate checks may be triggered by detected network errors

---

## Shutdown Behavior

- Monitors run indefinitely (until manually stopped with SIGINT/SIGTERM)
- On SIGTERM/interrupt:
  - Active downloads continue to completion (graceful shutdown)
  - Monitor goroutines detect signal and exit main loop
  - Main thread waits for all monitor goroutines (via `WaitGroup.Wait()`)
  - All goroutines clean up their resources before exiting
  - Program exits cleanly (exit code 0)
  - Lockfiles remain if downloads were in progress; will be detected on next app startup

## Summary: Core Workflow

```
1. Load Configs (Global + Platform-specific)
   └─ Validate at least one platform is enabled

2. Initialize Systems
   ├─ Create download semaphore (global buffered channel)
   ├─ Clean old lockfiles if clear_all_lockfiles is enabled
   ├─ Spawn Global Connection Monitor (singleton - background)
   │  └─ Check connectivity every 10s, with immediate checks triggered by network errors
   └─ Spawn system goroutine for update checks

3. Spawn Monitors (YouTube + Twitch in parallel)
   ├─ For each monitor:
   │  ├─ Load platform archive file into memory (if enabled)
   │  ├─ Create working directory
   │  ├─ Subscribe to global connection monitor
   │  ├─ Start polling loop (check every {poll_interval})
   │  │  ├─ Check global connection status (block if offline via pauseCond.Wait())
   │  │  ├─ Calculate request spacing (freshness target vs RPS safety limit)
   │  │  ├─ Stagger requests with jitter (±10%)
   │  │  ├─ Fetch live status from API/RSS (with rate limiting)
   │  │  ├─ Apply regex filters
   │  │  └─ Track state changes in liveStatus map
   │  │
   │  └─ Start download manager (every 5 seconds)
   │     └─ For each live channel:
   │        ├─ Check global connection status (block if offline)
   │        ├─ Check archive (skip if already downloaded)
   │        ├─ Check session cache (skip if already in this session)
   │        ├─ Check for lockfile (skip if download already in progress)
   │        ├─ Acquire semaphore slot
   │        ├─ Launch downloader subprocess
   │        └─ Spawn waitForDownload() goroutine
   │
   └─ waitForDownload() goroutine (per download):
      ├─ Wait for process exit
      ├─ Release semaphore slot
      ├─ Delete lockfile
      ├─ Append video ID to platform archive file (if success)
      ├─ Update session cache
      └─ Close logger and clean up

4. Global Connection Monitor (singleton, runs once):
   ├─ Rotate through 4 reliable DNS hosts (with fallback)
   ├─ Apply hysteresis (3-threshold) to prevent flapping
   ├─ Broadcast state changes to all subscribed monitors via pauseCond.Broadcast()
   ├─ Track lastLogged to prevent duplicate logs
   └─ Use immediate trigger channel for faster checks after network errors

5. Main thread waits for all monitors to complete

6. On SIGINT/SIGTERM:
   ├─ Monitors exit main loops gracefully
   ├─ Downloads continue to completion in background
   ├─ All goroutines clean up resources
   └─ Program exits
```
