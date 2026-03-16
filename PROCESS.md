# StreamMon - Application Process Flow

StreamMon is an automated orchestration tool that monitors YouTube and Twitch channels, detects live streams, applies configurable filters, and automatically archives them using specialized downloaders. This document outlines the complete process flow for both platforms.

---

## Application Startup

### 1. Configuration Loading

When the application starts (`main.go`):

1. **Environment Setup**
   - Set terminal title to "StreamMon"
   - Clear console and print ASCII banner

2. **Load Global Configuration** (`configs/config.toml`)
   - Timezone setting
   - `max_concurrent_downloads`: Maximum simultaneous download threads
   - Platform enable flags: `enable_youtube`, `enable_twitch`
   - Debug flags: `youtube_verbose_debug`, `twitch_verbose_debug`
   - Logging: `save_download_logs`
   - If loading fails, use sensible defaults (UTC, 10 concurrent downloads, both enabled)

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
   - Calls `CheckChannelStatus()` for YouTube (TODO: RSS feed parsing)
   - Currently returns `IsLive: false` (implementation pending)
   - When implemented:
     - Construct RSS feed URL: `https://www.youtube.com/feeds/videos.xml?channel_id={ID}`
     - Parse XML for latest `<entry>`
     - Check `yt:liveBroadcastContent` for "live" or "upcoming" status
     - Compare video ID against previous state
     - Check if video is older than `ignore_older_than` setting

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

   c. **Lockfile Check**
   - Generate lockfile path: `.lock-{sanitized_channel_name}-{videoID}`
   - Check if lockfile exists (indicates previous/concurrent download)
   - If yes, release semaphore slot and skip

3. **Launch Download** (`launchDownloader()`)

   a. **Create Lockfile**
   - Create `.lock-{sanitized_channel_name}-{videoID}` file
   - If creation fails, release slot and return

   b. **Create Channel Directory**
   - Create `{working_directory}/{sanitized_channel_name}/` if needed
   - Download will execute in this directory

   c. **Setup Logging** (if `save_download_logs: true`)
   - Create debug log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{videoID}.debug.log` (contains all subprocess output)
   - Create regular log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{videoID}.log` (contains important events)
   - Redirect subprocess stdout/stderr to debug log file
   - Important events written to both regular and debug logs

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

### 4. Download Completion

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`

2. **Release Resources** (immediately upon exit)
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile
   - Close log file if exists

3. **Log Completion**
   - If error: Log "[YT] Download for {channel} finished with error: {error}"
   - If success: Log "[YT] Download for {channel} finished successfully."

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

   c. **Lockfile Check**
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
   - Create debug log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{broadcastID}.debug.log` (contains all subprocess output)
   - Create regular log file: `{channel_dir}/{date_created}-{sanitized_channel_name}-{broadcastID}.log` (contains important events)
   - Redirect subprocess stdout/stderr to debug log file
   - Important events written to both regular and debug logs

   d. **Build Downloader Command**
   - Twitch uses `twitch-dlp` via npm/npx
   - Command: `npx -y twitch-dlp [args from config] https://www.twitch.tv/{channelID}` (-y/--yes automatically updates)
   - Working directory: channel-specific directory

   e. **Start Process**
   - Execute command via `cmd.Start()` (non-blocking)
   - If start fails, delete lockfile, release slot, and return
   - Log: "[Twitch] Started download for {channel}: {title}"

   f. **Track Process**
   - Store process info in `activeDownloads[channelID]`
   - Spawn goroutine `waitForDownload()` to monitor completion

### 4. Download Completion

**In `waitForDownload()` goroutine:**

1. **Wait for Process Exit**
   - Block on `cmd.Wait()`

2. **Release Resources** (immediately upon exit)
   - Release download semaphore slot: `<-downloadSlots`
   - Remove from active download tracking
   - Delete lockfile
   - Close log file if exists

3. **Log Completion**
   - If error: Log "[Twitch] Download for {channel} finished with error: {error}"
   - If success: Log "[Twitch] Download for {channel} finished successfully."

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

### 3. Deduplication via Lockfiles

- **Lockfile Path**: `.lock-{sanitized_channel_name}-{videoID/broadcastID}`
- **Location**: Working directory
- **Purpose**: Prevent same stream from being downloaded twice
- **Lifecycle**: Created before launch, deleted after process exit

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

- All important events (download start/completion)
- All subprocess output from twitch-dlp/yt-dlp (every line captured)
- Download errors and state transitions
- Download progress updates
- Format: `{channel_dir}/{date_created}-{sanitized_channel_name}-{videoID}.log`

#### Terminal Output

**Always shown:**

- Monitor startup/shutdown
- Channel status transitions ("is now LIVE", "has gone OFFLINE")
- Download start/completion with title and status
- Important errors and warnings
- Download progress reports

**Conditional (debug flags control visibility):**

- If `twitch_api_verbose_debug: true`: Twitch GraphQL API calls and responses
- If `twitch_dlp_verbose_debug: true`: Raw twitch-dlp subprocess output
- If `youtube_dlp_verbose_debug: true`: Raw yt-dlp subprocess output
- If `{platform}_verbose_debug: true`: All debug output for that platform (fallback)

#### Terminal Colors

- **Colored Terminal Output**: Different colors for YouTube (red), Twitch (purple), info (blue), debug (cyan)
- **Timestamps**: All terminal output includes timestamp with configurable timezone
- **Subprocess lines** tagged with `[SUBPROCESS]`
- **Progress updates** tagged with `[PROGRESS]`
- **Debug lines** tagged with `[DEBUG]`

---

## Configuration Files

### Global Config (`config.toml`)

```toml
timezone = "UTC"
max_concurrent_downloads = 10
enable_youtube = true
enable_twitch = true
youtube_verbose_debug = false
twitch_verbose_debug = false
save_download_logs = true
```

### YouTube Config (`config_yt.toml`)

```toml
[streammon]
working_directory = "download_yt"
args = ["--format", "best", ...]

[scraper.rss]
poll_interval = "60s"
ignore_older_than = "7d"

[[channel]]
id = "UC..."
name = "Channel Name"
filters = ["(?i).*karaoke.*"]
```

### Twitch Config (`config_twitch.toml`)

```toml
[streammon]
working_directory = "download_twitch"
args = ["--format", "best", ...]

[scraper]
poll_interval = "30s"

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
