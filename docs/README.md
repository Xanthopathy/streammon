# streammon

Monitors YouTube and Twitch channels for live streams, applies regex filters, and automatically downloads them with `yt-dlp` and `twitch-dlp`.

## Features

- **Multi-Platform Monitoring**: Concurrently watch channels on YouTube and Twitch.
- **Advanced Filtering**: Use regular expressions to download only the streams you want based on their titles.
- **Concurrent Downloads**: Download multiple streams at once, up to a global limit you define.
- **Robust Deduplication**: A multi-layer system prevents re-downloading the same stream:
  - **Archive File**: Remembers successfully downloaded video IDs across restarts.
  - **Session Cache**: Remembers downloads within the current session.
  - **Lockfiles**: Prevents multiple instances from downloading the same stream.
- **Connection Stability**: Automatically pauses all monitoring when your internet connection drops and gracefully resumes when it's restored, preventing log spam and errors.
- **Fallback Checking (YouTube)**: If the primary check method (e.g., RSS) fails or reports a channel as offline, it automatically tries a secondary method (e.g., scraping the `/live` page) to ensure streams aren't missed.

## Requirements

- **yt-dlp**: Must be in `PATH`
- **twitch-dlp**: Installed via npm (npx)
- **FFmpeg**: Required by yt-dlp
- **Node.js**: Required for twitch-dlp and yt-dlp's JavaScript runtime
- **Go 1.21+**: Only if building from source

## Setup

### Step 1: Get the Files

**Option A: GitHub Release**

- Download the `.zip` of your respective platform from the [release page](https://github.com/Xanthopathy/streammon/releases)
- Extract it
- Run the binary for your OS:
  - **Windows**: `streammon.exe`
  - **Linux**: `./streammon`
  - **macOS (Intel)**: `./streammon-macos`
  - **macOS (Apple Silicon)**: `./streammon-macos-arm64`

**Option B: Build from Source**

- Clone/download the full project

### Step 2: Configure

Edit the `configs/` folder:

- `streammon_config.toml` - Global settings (timezone, concurrent downloads, debug flags)
- `streammon_config_yt.toml` - YouTube channels and filters
- `streammon_config_twitch.toml` - Twitch channels and filters

#### Add a Channel

```toml
[[channel]]
id = "UCFzQd4pZ43ZNEdWBFe7QOKA"
name = "Saya Sairroxs"
filters = ["(?i).*karaoke.*"]
```

For YouTube: `id` is the channel ID (from https://www.youtube.com/channel/UC...)  
For Twitch: `id` is the channel login name (from https://www.twitch.tv/...)  
`filters` is optional; leave empty or omit to download all streams

### Step 3: Run

**From Release:**

- **Windows**: Double-click `streammon.exe` or run in terminal: `streammon.exe`
- **Linux**: Run in terminal: `./streammon`
- **macOS (Intel)**: Run in terminal: `./streammon-macos`
- **macOS (Apple Silicon)**: Run in terminal: `./streammon-macos-arm64`

**From Source (Windows):**

```
.\build.ps1
```

or

```
go run .\cmd\streammon\main.go
```

**From Source (Linux/Mac):**

```
./build.ps1
```

or

```
go run ./cmd/streammon/main.go
```

### Output

- Downloads go to `download_yt/` and `download_twitch/` by default
- Logs saved to `{channel_name}/{date}-{title}-{id}.log` if enabled in config
- Check terminal output for status

## Configuration Reference

### config.toml

| Setting                        | Default | Description                                                                                 |
| ------------------------------ | ------- | ------------------------------------------------------------------------------------------- |
| `timezone`                     | `UTC`   | Timezone for logs. Use IANA names (`Asia/Tokyo`) or offsets (`UTC-1`, `UTC+1`).             |
| `max_concurrent_downloads`     | `10`    | Max simultaneous downloads across all platforms.                                            |
| `enable_youtube`               | `true`  | Enable the YouTube monitor.                                                                 |
| `enable_twitch`                | `true`  | Enable the Twitch monitor.                                                                  |
| `save_download_logs`           | `true`  | Save detailed logs from `yt-dlp`/`twitch-dlp` to files in the channel's download directory. |
| `subprocess_progress_interval` | `30`    | Throttle `[download]` progress lines in logs (seconds). Set to `0` to log every update.     |
| `subprocess_wait_interval`     | `600`   | Throttle `[wait]` retry lines in logs (seconds).                                            |
| `youtube_archive_downloads`    | `true`  | Save downloaded YouTube video IDs to `archive.txt` to prevent re-downloads across restarts. |
| `twitch_archive_downloads`     | `true`  | Save downloaded Twitch video IDs to `archive.txt`.                                          |
| `clear_all_lockfiles`          | `true`  | Automatically delete any leftover `.lock` files on startup to prevent issues after a crash. |
| `youtube_verbose_debug`        | `true`  | General toggle for showing non-essential YouTube monitor logs in the terminal.              |
| `twitch_verbose_debug`         | `true`  | General toggle for showing non-essential Twitch monitor logs in the terminal.               |
| `youtube_api_verbose_debug`    | `true`  | Show detailed YouTube API (RSS, /live page) request/response logs.                          |
| `twitch_api_verbose_debug`     | `false` | Show detailed Twitch GQL API request/response logs.                                         |
| `youtube_dlp_verbose_debug`    | `true`  | Show raw `yt-dlp` subprocess output in the terminal.                                        |
| `twitch_dlp_verbose_debug`     | `true`  | Show raw `twitch-dlp` subprocess output in the terminal.                                    |

### config_yt.toml

| Setting                   | Description                                                                                                                                                  |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `working_directory`       | Where to save YouTube downloads.                                                                                                                             |
| **`[scraper]` section**   |                                                                                                                                                              |
| `check_method`            | Default method to check for streams. Options: `"rss"` (low-bandwidth, can be delayed) or `"live"` (more accurate, heavier). The other is used as a fallback. |
| `ignore_older_than`       | Prevents downloading old videos that reappear in the RSS feed.                                                                                               |
| `poll_interval`           | How often to check for new streams (e.g., `60s`). This is a "freshness target" for the whole channel list.                                                   |
| `fallback_duration`       | How long to stay on the fallback method after a failure (e.g., `15m`) before retrying the primary method.                                                    |
| `max_requests_per_second` | Hard rate limit for API calls (default: `2`). This acts as a safety to prevent being flagged as a bot.                                                       |
| `args`                    | Arguments passed to `yt-dlp`.                                                                                                                                |

### config_twitch.toml

| Setting                   | Description                                       |
| ------------------------- | ------------------------------------------------- |
| `working_directory`       | Where to save Twitch downloads.                   |
| **`[scraper]` section**   |                                                   |
| `poll_interval`           | How often to check for new streams (e.g., `30s`). |
| `max_requests_per_second` | Hard rate limit for API calls (default: `2`).     |
| `args`                    | Arguments passed to `twitch-dlp`.                 |

## How It Works

### Core Loop

6. **Polls Channels**: Each monitor (YouTube, Twitch) runs a continuous loop, checking all its configured channels every `poll_interval`.
7. **Spreads Requests**: To avoid being detected as a bot, requests are not sent in a single burst. They are spaced out based on your `poll_interval` and `max_requests_per_second` settings.
8. **Checks Status**: For each channel, it uses the configured method (`rss` or `live` for YouTube, GQL for Twitch) to see if a stream is live. If the primary method fails, it tries a fallback.
9. **Applies Filters**: If a stream is live, its title is checked against your list of regex `filters`. If it doesn't match, it's ignored.
10. **Queues for Download**: If a stream is live and passes the filters, it's queued for download.
11. **Manages Downloads**: A separate manager process constantly checks the queue and starts new downloads as soon as a slot is free, up to the `max_concurrent_downloads` limit.
12. **Logs Everything**: The application provides detailed logs for status changes, download progress, and errors, both in the terminal and in log files (if enabled).
