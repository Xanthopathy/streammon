# StreamMon

StreamMon is an automated orchestration tool written in **Go** designed to monitor YouTube and Twitch channels, detect new/live content, apply configurable filters via regex, and automatically archive them using `yt-dlp` and `twitch-dlp`.

It is designed to archive streams that might get deleted or content that may be edited or removed immediately after broadcast.

## Features

### Core Functionality

- **Dual Platform Support**: Monitors both YouTube and Twitch channels simultaneously.
- **Lightweight Polling Architecture**:
  - **YouTube**: Polls RSS feeds every `poll_interval` seconds (default 120s) for immediate detection of new video entries with zero API overhead.
  - **Twitch**: Queries GraphQL API every `poll_interval` seconds (default 120s) for real-time live status detection.
  - **Intelligent Deduplication**: Two-layer deduplication prevents re-downloading the same stream within an app instance and across restarts.
- **Selective Archiving**:
  - **Regex Filtering**: Define allow-lists per channel (e.g., download `(?i).*karaoke.*` but ignore `(?i).*gaming.*`).
  - **Session-Based Deduplication**: In-memory cache prevents redundant downloads of the same stream in the same session.
  - **Lockfile Persistence**: Survives app crashes and prevents corruption from concurrent downloads.
- **Process Management**:
  - **Concurrency Limits**: Configurable slots (default 10 simultaneous downloads) to prevent bandwidth saturation.
  - **Process Isolation**: Each download runs in a separate subprocess; monitor survives downloader crashes.
  - **Throttled Progress Logging**: Configurable interval to reduce spam while maintaining visibility into active downloads.

### Configuration

Configuration is managed via TOML files:

- **Global Settings** (`config.toml`): Working directories, check intervals, concurrency limits, debug flags.
- **Platform-Specific** (`config_yt.toml`, `config_twitch.toml`): Per-channel configuration, filters, downloader arguments.

## Dependencies

### Core

- **Go 1.21+**: The application runtime.
- **yt-dlp**: Downloads YouTube streams/VODs and extracts metadata. Must be in `PATH`.
- **twitch-dlp**: Specialized downloader for Twitch streams. Invoked via `npx twitch-dlp`.
- **FFmpeg**: Required by yt-dlp for merging video/audio streams and processing thumbnails.
- **Node.js**: Required for running `twitch-dlp` via npm and for yt-dlp's JavaScript runtime.

### Optional

- **ffprobe**: Recommended for video inspection and validation (usually bundled with FFmpeg).

## Usage

```bash
# Run from source
go run ./cmd/streammon/main.go

# Or build and run the binary
go build -o streammon.exe ./cmd/streammon
./streammon.exe
```

### Configuration Files

1. **Edit** `configs/config.toml` to set global settings (timezone, concurrency, debug flags).
2. **Edit** `configs/config_yt.toml` to add YouTube channels, filters, and yt-dlp arguments.
3. **Edit** `configs/config_twitch.toml` to add Twitch channels, filters, and twitch-dlp arguments.
4. **Run** the application.

### Example Channel Configuration

```toml
[[channel]]
id = "UCg7sW-h1PUowdiR5K4HlBew"
name = "Example Streamer"
filters = ["(?i).*karaoke.*|.*archive.*"]  # Download only karaoke and archive streams
```

## Architecture

### Monitoring Loop

1. **Polling**: Every `poll_interval` seconds, check each platform for live/new content.
2. **Filtering**: Apply regex patterns to titles; skip if no match.
3. **Deduplication Check**:
   - Session cache (in-memory): Was this already downloaded in this app instance?
   - Lockfiles: Was this interrupted by a crash?
4. **Download Manager**: Every 5 seconds, launch pending downloads up to the concurrency limit.

### Deduplication Layers

**Layer 1 - Session Cache** (temporary, cleared on restart):

- Tracks successfully downloaded videos in memory.
- Prevents re-launching yt-dlp for the same VideoID if it completed in this session.

**Layer 2 - Lockfiles** (persistent):

- Created before download launch, deleted after completion.
- Survives app crashes and prevents corruption from concurrent access.
- Acts as a safety net for Layer 1.

### Logging

- **Log Files**: Stored in per-channel directories under the working directory.
  - Created only if `save_download_logs: true` in `config.toml`.
  - Contains all subprocess output (yt-dlp/twitch-dlp) and important events.
  - Progress lines throttled by `subprocess_progress_interval` config.

- **Terminal Output**:
  - Always shows: Channel status changes, download start/completion, errors.
  - Conditional visibility controlled by debug flags:
    - `youtube_dlp_verbose_debug`: Show raw yt-dlp output in terminal.
    - `twitch_dlp_verbose_debug`: Show raw twitch-dlp output in terminal.
    - `youtube_api_verbose_debug`: Show YouTube API calls.
    - `twitch_api_verbose_debug`: Show Twitch API calls.

## Roadmap & Future Work

### Completed ✓

- [x] YouTube monitoring via RSS feed parsing
- [x] Twitch monitoring via GraphQL API
- [x] Regex-based filtering
- [x] Multi-layer deduplication (session cache + lockfiles)
- [x] Output progress throttling
- [x] Concurrent download management with semaphore
- [x] Debug logging infrastructure
- [x] Go rewrite from Python

### Planned

- [ ] Persistent download tracking (survives app restarts)
- [ ] TUI dashboard for real-time monitoring
- [ ] Webhook notifications on download completion
- [ ] Integration with cloud storage (e.g., Rclone)
- [ ] Docker containerization for 24/7 deployments

## Project Structure

```
streammon/
├── cmd/streammon/main.go          # Entry point
├── internal/
│   ├── config/config.go           # Config parsing
│   ├── monitor/
│   │   ├── base_monitor.go        # Shared orchestration logic
│   │   ├── youtube.go             # YouTube-specific logic
│   │   ├── youtube_api.go         # YouTube RSS fetching
│   │   ├── twitch.go              # Twitch-specific logic
│   │   ├── twitch_api.go          # Twitch GraphQL API
│   │   └── monitor.go             # Shared interfaces/types
│   └── util/
│       ├── logger.go              # Logging infrastructure
│       └── utils.go               # Utility functions (lockfiles, etc.)
├── configs/
│   ├── config.toml                # Global settings
│   ├── config_yt.toml             # YouTube channels
│   └── config_twitch.toml         # Twitch channels
├── testing/                       # Test utilities
└── _old/                          # Legacy Python implementation
```
