# StreamMon

StreamMon is an automated orchestration tool designed to monitor YouTube and Twitch channels, detect live streams based on configurable filters, and automatically archive them using `yt-dlp` and `twitch-dlp`.

It is designed to archive streams that might get deleted (unarchived) or edited immediately after broadcast.

## Features

### Core Functionality

- **Dual Platform Support**: Monitors both YouTube and Twitch channels simultaneously.
- **Smart Polling Architecture**:
  - **Fast Track (RSS)**: Polls YouTube RSS feeds every 60 seconds for immediate detection of new video entries with minimal API overhead.
  - **Slow Track (Deep Check)**: Periodically verifies channel status to catch streams that RSS might miss or to confirm "Waiting Room" status.
  - **Rate Limit Handling**: Detects HTTP 429 errors and automatically backs off to prevent IP bans.
- **Selective Archiving**:
  - **Regex Filtering**: Define allow-lists per channel (e.g., download `(?i).*karaoke.*` but ignore `(?i).*gaming.*`).
  - **Deduplication**: Uses lockfiles to ensure the same stream is not downloaded twice.
- **Process Management**:
  - **Concurrency Limits**: Configurable slots (e.g., max 10 downloads) to prevent bandwidth saturation.
  - **Process Isolation**: Spawns downloads in separate shell windows/processes; if a download crashes, the monitor survives.

### Configuration

Configuration is managed via a central `config.toml` file:

- **Global Settings**: Working directories, check intervals, and quality preferences.
- **Channel Management**: Add/Remove channels, assign specific regex filters, and set custom output paths.

## Dependencies

### Core

- **yt-dlp**: The heavy lifter for YouTube downloading and metadata extraction.
- **twitch-dlp**: Specialized downloader for Twitch streams (handles ads/segments better than generic tools).
- **FFmpeg**: Required for merging video/audio streams and processing thumbnails.
- **Node.js**: Required runtime for `yt-dlp` to execute complex JavaScript challenges from YouTube.

### Optional / Legacy

- **Streamlink**: Currently used for Twitch status checking (slated for deprecation in favor of lightweight API checks).

## Roadmap & Todo

### Optimization

- [ ] **Lightweight Pings**: Replace spawning heavy `yt-dlp` subprocesses for status checks with lightweight HTTP requests (checking `302 Redirects` on YouTube `/live` URLs).
- [ ] **Unified Logic**: Merge `twitchmon.py` logic into the main orchestrator to use the shared `config.toml`.

### User Interface

- [ ] **TUI Dashboard**: Implement a dual-pane terminal interface (using `Rich` or `Textual`).
  - **Left Pane**: Scrolling history of orchestrator logs.
  - **Right Pane**: Dynamic grid showing real-time progress bars and stdout for active downloads.

### Cross-Platform

- [ ] **Remove Batch Dependency**: Refactor `.bat` file logic into native code to support Linux/Docker environments without Wine.

## Usage

1. **Configure**: Edit `config.toml` to add channels and filters.
2. **Run**:
   ```bash
   python ytmon.py
   ```

### Insights & Recommendations

#### 1. Is `streamlink` needed?

**No, it is not strictly needed.**
Currently, your `twitchmon.py` spawns a `streamlink` process just to check if a channel is live. This is expensive (CPU-wise).

- **Alternative 1 (Existing Tool):** You can use `yt-dlp --dump-json https://twitch.tv/user` to check status, just like you do for YouTube.
- **Alternative 2 (Lightweight - Recommended):** Twitch has a GQL (GraphQL) API that is easy to query via a standard HTTP POST request. This returns a JSON object indicating if the user is live. This requires no external process spawning and is milliseconds fast.

#### 2. Should you switch from Python?

If your primary concern is the "Heavy Loop" killing your CPU, switching languages will help, but **changing your logic** will help more.

- **The Problem:** Currently, you spawn a heavy subprocess (`yt-dlp.exe` or `streamlink.exe`) for _every_ channel check. If you monitor 50 channels, that's 50 heavy process startups every loop.
- **The Fix (Python):** Use the `aiohttp` library (which you already imported) to check the status via HTTP headers or API calls. Only spawn `yt-dlp` when you confirm a download is needed.
- **The Switch (Go/Golang):** If you want to rewrite, **Go** is the best choice for this application.
  - **Concurrency:** Go's "Goroutines" are incredibly cheap. You can monitor 500 channels concurrently with negligible CPU usage.
  - **Single Binary:** You can compile the app into a single `.exe` that includes your TUI and logic, with no need to manage Python environments or batch files.
  - **Cross-Platform:** Go handles process spawning on Windows and Linux natively, removing the need for your `.bat` files.

**Recommendation:**
If you stick with Python, implement the **Lightweight Ping** todo item immediately. If you want a robust, distributable app that runs 24/7 with 1% CPU usage, rewrite in **Go**.
