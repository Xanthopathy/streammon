# streammon

Monitors YouTube and Twitch channels for live streams, applies regex filters, and automatically downloads them with `yt-dlp` and `twitch-dlp`.

## Requirements

- **yt-dlp**: Must be in `PATH`
- **twitch-dlp**: Installed via npm (npx)
- **FFmpeg**: Required by yt-dlp
- **Node.js**: Required for twitch-dlp and yt-dlp's JavaScript runtime
- **Go 1.21+**: Only if building from source

## Setup

### Step 1: Get the Files

**Option A: GitHub Release**

- Download the `.zip` of your respective platform from the release
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

| Setting                        | Default | Description                                                           |
| ------------------------------ | ------- | --------------------------------------------------------------------- |
| `timezone`                     | `UTC`   | Timezone for logs. Use IANA names (`Asia/Tokyo`) or offsets (`UTC+7`) |
| `max_concurrent_downloads`     | `10`    | Max simultaneous downloads                                            |
| `enable_youtube`               | `true`  | Monitor YouTube                                                       |
| `enable_twitch`                | `true`  | Monitor Twitch                                                        |
| `save_download_logs`           | `true`  | Save logs to files                                                    |
| `subprocess_progress_interval` | `10`    | Throttle [download] lines in logs (seconds)                           |
| `subprocess_wait_interval`     | `60`    | Throttle [wait] lines in logs (seconds)                               |
| `youtube_verbose_debug`        | `true`  | Show YouTube monitor debug output                                     |
| `twitch_verbose_debug`         | `true`  | Show Twitch monitor debug output                                      |
| `youtube_api_verbose_debug`    | `false` | Show YouTube API calls                                                |
| `twitch_api_verbose_debug`     | `false` | Show Twitch API calls                                                 |
| `youtube_dlp_verbose_debug`    | `true`  | Show yt-dlp output in terminal                                        |
| `twitch_dlp_verbose_debug`     | `true`  | Show twitch-dlp output in terminal                                    |

### config_yt.toml & config_twitch.toml

| Setting             | Description                                       |
| ------------------- | ------------------------------------------------- |
| `working_directory` | Where to save downloads                           |
| `poll_interval`     | How often to check for new streams (e.g., `120s`) |
| `args`              | Arguments passed to yt-dlp/twitch-dlp             |

## How It Works

1. Polls each platform every `poll_interval`
2. Checks if new/live content matches your filters
3. Prevents duplicate downloads (same video not downloaded twice in one session)
4. Downloads up to `max_concurrent_downloads` at once
5. Logs progress and errors
