# StreamMon - Application Process Flow

StreamMon is an automated orchestration tool that monitors YouTube and Twitch channels, detects live streams, applies configurable filters, and automatically archives them using specialized downloaders. This document outlines the complete process flow for both platforms.

---

## Application Startup

### 1. Configuration Loading

When the application starts (`main.go`):

1. **Environment Setup**
   - Set terminal title to "streammon" (lowercase)
   - Clear console and print ASCII banner

2. **Load Global Configuration** (`configs/config.toml`)
   - Timezone setting: `timezone`
   - `max_concurrent_downloads`: Maximum simultaneous download threads
   - `subprocess_progress_interval`: Throttle [download] progress lines (seconds)
   - `subprocess_wait_interval`: Throttle [wait] progress lines (seconds)
   - Platform enable flags: `enable_youtube`, `enable_twitch`
   - Debug flags: `youtube_verbose_debug`, `twitch_verbose_debug`
   - Logging: `save_download_logs`
   - If loading fails, use sensible defaults: UTC timezone, 10 concurrent downloads, both platforms enabled, 10s download throttle, 60s wait throttle, logging enabled

3. **Load Platform-Specific Configurations**
   - **YouTube**: Load `configs/config_yt.toml` if `enable_youtube` is true
   - **Twitch**: Load `configs/config_twitch.toml` if `enable_twitch` is true
   - If a config file is missing/invalid, that platform is disabled with a warning

4. **Validation**
   - Ensure at least one platform is enabled and configured
   - If both are disabled/failed to load, exit with fatal error

### 2. Initialization

1. **Create Working Directory**
   - Each monitor creates its configured `working_directory` if it doesn't exist
   - Subdirectories are created per channel as needed

2. **Initialize Download Semaphore**
   - A global semaphore (buffered channel) is created with capacity = `max_concurrent_downloads`
   - This limits concurrent downloads across both platforms

3. **Start Monitors**
   - If YouTube is enabled, spawn `MonitorYouTube()` in a goroutine
   - If Twitch is enabled, spawn `MonitorTwitch()` in a goroutine
   - Main thread waits for all monitors to complete (via `sync.WaitGroup`)

---

## YouTube Monitoring Process

### 1. Monitor Initialization

The YouTube monitor (`youtube.go` → `base_monitor.go`):

1. Creates a `YTMonitor` instance wrapping a `BaseMonitor`
2. Retrieves YouTube-specific configuration:
   - RSS poll interval from `config_yt.toml`
   - List of channels to monitor
   - yt-dlp arguments and working directory
3. Prints startup log with channel count and working directory

### 2. Continuous Polling Loop

**Frequency**: Every `poll_interval` (e.g., 60 seconds) in `config_yt.toml`

1. **Initial Check**: Immediately upon startup
2. **Recurring Checks**: Every polling interval

#### Channel Status Check (`checkAllChannels()` → `checkChannel()`)

For each configured YouTube channel:

1. **Fetch Live Status**
   - Calls `CheckChannelStatus()` for YouTube → `CheckLiveYouTube()`
   - Constructs RSS feed URL: `https://www.youtube.com/feeds/videos.xml?channel_id={ID}`
   - Parses XML for latest `<entry>` element
   - Extracts video ID from `<id>` and upload timestamp from `<updated>`
   - Checks if video's `<updated>` timestamp is within `ignore_older_than` duration
   - If yes, marks as live and returns video metadata
   - If no, returns offline status

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

   a. **Global Capacity Check**
   - Attempt to acquire a slot from the download semaphore
   - If all slots are in use, skip this channel (retry next manager cycle)

   b. **Local Download Check**
   - Check if a download for this channel is already running
   - If yes, skip

   c. **Session Cache Check** (NEW)
   - Check if this video was already downloaded in this app instance
   - Maintains in-memory map: `downloadedVideos[channelID][videoID]`
   - If video is in cache, skip (prevents redundant downloads of same video in same session)
   - This survives across polling cycles but resets when app restarts

   d. **Lockfile Check**
   - Check if lockfile exists: `.lock-{sanitized_channel_name}-{videoID}`
   - If yes, release semaphore slot and skip

3. **Launch Download** (`launchDownloader()`)

   a. **Create Lockfile**
   - Create `.lock-{sanitized_channel_name}-{videoID}` file
   - If creation fails, release slot and return

   b. **Create Channel Directory**
   - Create `{working_directory}/{sanitized_channel_name}/` if needed
   - Download will execute in this directory

   c. **Setup Logging** (if `save_download_logs: true`)
   - Create single log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{videoID}.log`
   - Redirects subprocess stdout/stderr to be captured and logged
   - Subprocess output is written to log file with throttling applied:
     - `[download]` progress lines throttled by `subprocess_progress_interval` (10s default)
     - `[wait]` lines throttled by `subprocess_wait_interval` (60s default)
   - Subprocess output visibility in terminal controlled by `*_dlp_verbose_debug` flags (independent of file logging)

   d. **Build Downloader Command**
   - YouTube uses `yt-dlp`
   - Command: `yt-dlp [args from config] https://www.youtube.com/watch?v={videoID}`
   - Working directory: channel-specific directory

   e. **Start Process**
   - Execute command via `cmd.Start()` (non-blocking)
   - If start fails, delete lockfile, release slot, and return
   - Log: "[YT] Started download for {channel}: {title}"

   f. **Track Process**
   - Store process info in `activeDownloads[channelID]`
   - Spawn goroutine `waitForDownload()` to monitor completion

### 4. Download Completion (YouTube)

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`

2. **Reset Terminal Title**
   - Reset title to "streammon" when process completes

3. **Release Resources** (immediately upon exit)
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile
   - Close log file if exists

4. **Log Completion**
   - If error: Log "[YT] Download for {channel} finished with error: {error}"
   - If success: Log "[YT] Download for {channel} finished successfully."

5. **Mark as Downloaded** (on success)
   - Add video ID to session cache: `downloadedVideos[channelID][videoID] = true`
   - Prevents re-downloading the same video in subsequent polling cycles

### 5. Safety Net Logic

**After API reports stream as offline:**

- If a download is **actively running** for the same stream ID, **ignore** the offline signal
- This prevents premature abortion of downloads due to API lag or false negatives
- Safety check is bypassed only if download already in progress

---

## Twitch Monitoring Process

### 1. Monitor Initialization

The Twitch monitor (`twitch.go` → `base_monitor.go`):

1. Creates a `TwitchMonitor` instance wrapping a `BaseMonitor`
2. Retrieves Twitch-specific configuration:
   - Poll interval from `config_twitch.toml`
   - List of channels to monitor
   - twitch-dlp arguments and working directory
3. Prints startup log with channel count and working directory

### 2. Continuous Polling Loop

**Frequency**: Every `poll_interval` (e.g., 30 seconds) in `config_twitch.toml`

1. **Initial Check**: Immediately upon startup
2. **Recurring Checks**: Every polling interval

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

   a. **Global Capacity Check**
   - Attempt to acquire a slot from the download semaphore
   - If all slots are in use, skip this channel (retry next manager cycle)

   b. **Local Download Check**
   - Check if a download for this channel is already running
   - If yes, skip

   c. **Session Cache Check** (NEW)
   - Check if this broadcast/stream was already downloaded in this app instance
   - Maintains in-memory map: `downloadedVideos[channelID][broadcastID]`
   - If stream is in cache, skip (prevents redundant downloads)
   - This survives across polling cycles but resets when app restarts

   d. **Lockfile Check**
   - Check if lockfile exists: `.lock-{sanitized_channel_name}-{broadcastID}`
   - If yes, release semaphore slot and skip

3. **Launch Download** (`launchDownloader()`)

   a. **Create Lockfile**
   - Create `.lock-{sanitized_channel_name}-{broadcastID}` file
   - If creation fails, release slot and return

   b. **Create Channel Directory**
   - Create `{working_directory}/{sanitized_channel_name}/` if needed
   - Download will execute in this directory

   c. **Setup Logging** (if `save_download_logs: true`)
   - Create single log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{broadcastID}.log`
   - Redirects subprocess stdout/stderr to be captured and logged
   - Subprocess output is written to log file with throttling applied:
     - `[download]` progress lines throttled by `subprocess_progress_interval` (10s default)
     - `[wait]` lines throttled by `subprocess_wait_interval` (60s default)
   - Subprocess output visibility in terminal controlled by `*_dlp_verbose_debug` flags (independent of file logging)

   d. **Build Downloader Command**
   - Twitch uses `twitch-dlp` via npm/npx
   - Command: `npx -y twitch-dlp [args from config] https://www.twitch.tv/{channelID}` (-y/--yes automatically updates)
   - Working directory: channel-specific directory

   e. **Start Process**
   - Execute command via `cmd.Start()` (non-blocking)
   - If start fails, delete lockfile, release slot, and return
   - Log: "[Twitch] Started download for {channel}: {title}"

### 4. Download Completion (Twitch)

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`

2. **Reset Terminal Title**
   - Reset title to "streammon" when process completes

3. **Release Resources** (immediately upon exit)
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile
   - Close log file if exists

4. **Log Completion**
   - If error: Log "[Twitch] Download for {channel} finished with error: {error}"
   - If success: Log "[Twitch] Download for {channel} finished successfully."

5. **Mark as Downloaded** (on success)
   - Add broadcast ID to session cache: `downloadedVideos[channelID][broadcastID] = true`
   - Prevents re-downloading the same broadcast in subsequent polling cycles

### 5. Safety Net Logic

**After API reports stream as offline:**

- If a download is **actively running** for the same broadcast ID, **ignore** the offline signal
- This prevents premature abortion of downloads due to API lag
- Safety check is bypassed only if download already in progress

---

## Key Architectural Patterns

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

- **Layer 1 - In-Memory Session Cache**: `downloadedVideos[channelID][videoID/broadcastID]`
  - Prevents re-launching downloads for videos already downloaded in this app instance
  - Temporary (cleared on app restart)
  - Supercedes lockfiles for same-session re-attempts

- **Layer 2 - Lockfiles**: `.lock-{sanitized_channel_name}-{videoID/broadcastID}`
  - Files in working directory
  - Persistence across app restarts (survives crashes)
  - Purpose: Handle app crashes, concurrent downloads, etc.
  - Lifecycle: Created before launch, deleted after process exit

### 4. State Monitoring

- **Live Status Map**: `map[channelID]LiveInfo`
- **Active Downloads Map**: `map[channelID]*downloadProcess`
- **Protected by Mutexes**:
  - `statusMutex`: Protects live status map
  - `downloadMutex`: Protects active downloads map

### 5. Process Isolation

- Each download runs in a separate subprocess
- Crash of one downloader doesn't crash monitor
- Crashes release their semaphore slots and lockfiles

### 6. Logging

#### Log File

**Single `.log` file** (created if `save_download_logs: true`):

- All subprocess output from twitch-dlp/yt-dlp (every line captured)
- Download progress lines throttled by `subprocess_progress_interval` config
- All important events (download start/completion, errors, state transitions)
- Format: `{channel_dir}/{date_created}-{sanitized_channel_name}-{videoID}.log`
- **SUBPROCESS lines tagged with**: `[SUBPROCESS] [{channel_name}] {output}`

#### Terminal Output

**Always shown:**

- Monitor startup/shutdown
- Channel status transitions ("is now LIVE", "has gone OFFLINE")
- Download start/completion with title and status
- Important errors and warnings
- Session cache hits ("already downloaded in this session")

**Conditional (debug flags control visibility):**

- If `youtube_verbose_debug: true`: YouTube monitor debug output
- If `twitch_verbose_debug: true`: Twitch monitor debug output
- If `youtube_api_verbose_debug: true`: YouTube RSS API calls and responses
- If `twitch_api_verbose_debug: true`: Twitch GraphQL API calls and responses
- If `youtube_dlp_verbose_debug: true`: Raw yt-dlp subprocess output (with throttling on [download] and [wait] lines)
- If `twitch_dlp_verbose_debug: true`: Raw twitch-dlp subprocess output (with throttling on [download] and [wait] lines)
- **Note:** DLP verbose flags control **terminal** printing only; log files always receive subprocess output with separate throttling per line type

#### Terminal Colors

- **Colored Terminal Output**: Different colors for YouTube (red), Twitch (purple), info (blue)
- **Timestamps**: All terminal output includes timestamp with configurable timezone (IANA timezone names or UTC offsets like UTC+7)
- **Subprocess lines** tagged with `[SUBPROCESS]` and throttled:
  - `[download]` lines: throttled by `subprocess_progress_interval` (default 10s)
  - `[wait]` lines: throttled by `subprocess_wait_interval` (default 60s)
- **Debug lines** colored cyan

---

## Configuration Files

### Global Config (`streammon_config.toml`)

```toml
timezone = "UTC"
max_concurrent_downloads = 10
enable_youtube = true
enable_twitch = true
save_download_logs = true
subprocess_progress_interval = 10     # Throttle [download] lines (seconds)
subprocess_wait_interval = 60         # Throttle [wait] lines (seconds)
youtube_verbose_debug = true
twitch_verbose_debug = true
youtube_api_verbose_debug = false
twitch_api_verbose_debug = false
youtube_dlp_verbose_debug = true
twitch_dlp_verbose_debug = true
```

### YouTube Config (`streammon_config_yt.toml`)

```toml
[streammon]
working_directory = "download_yt"
args = ["--wait-for-video", "60", "--live-from-start", ...]

[scraper]
poll_interval = "120s"
ignore_older_than = "24h"

[[channel]]
id = "UC..."
name = "Channel Name"
filters = ["(?i).*karaoke.*"]
```

### Twitch Config (`streammon_config_twitch.toml`)

```toml
[streammon]
working_directory = "download_twitch"
args = ["--live-from-start", "--retry-streams", "60", ...]

[scraper]
poll_interval = "120s"

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

- Missing config files: Use defaults, skip that platform
- Invalid poll intervals: Default to 60 seconds, log warning
- Invalid lockfile paths: Log error, skip download, release slot

### API Errors

- HTTP request failures: Log error, continue monitoring
- GraphQL/RSS parsing errors: Log error, continue monitoring
- Rate limiting (HTTP 429): Implicitly backed off by failing checks

### Download Errors

- Command start failure: Log error, delete lockfile, release slot
- Process crash: Log error, clean up resources, release slot
- Log file creation failure: Log warning, continue download without log file

---

## Shutdown Behavior

- Monitors run indefinitely (until manually stopped)
- On SIGTERM/interrupt:
  - Active downloads continue to completion
  - Monitor goroutines exit
  - Main thread waits for monitor goroutines (via `WaitGroup`)
  - Program exits cleanly

---

## Summary: Core Workflow

```
1. Load Configs (Global + Platform-specific)
2. Initialize Download Semaphore
3. Spawn Monitors (YouTube + Twitch in parallel)
   ├─ For each monitor:
   │  ├─ Start polling loop (check every {poll_interval})
   │  ├─ Start download manager (every 5 seconds)
   │  └─ For each check:
   │     ├─ Fetch live status from API/RSS
   │     ├─ Apply filters
   │     └─ Track state changes
   │
   └─ For each discovered live stream:
      ├─ Check capacity (acquire semaphore slot)
      ├─ Check for lockfile (dedup)
      ├─ Create lockfile
      ├─ Launch downloader subprocess
      ├─ Wait for completion asynchronously
      └─ Clean up (lockfile, slot, logs, process tracking)
4. Wait for monitors to complete
5. Exit
```
