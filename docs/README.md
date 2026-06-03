# streammon

streammon watches YouTube and Twitch channels, checks when they go live, and starts [yt-dlp](https://github.com/yt-dlp/yt-dlp) or [twitch-dlp](https://github.com/DmitryScaletta/twitch-dlp) for streams you actually want to save.

This is meant to replace the now deprecated [hoshinova](https://github.com/HoloArchivists/hoshinova). It can optionally use [livestream_dl](https://github.com/CanOfSocks/livestream_dl) as a fallback if the base `yt-dlp` download fails.

## What You Need

- `yt-dlp` in your `PATH`
- FFmpeg in your `PATH`
- Node.js, because Twitch downloads run through `npx twitch-dlp`
- Optional: [`livestream_dl`](https://github.com/CanOfSocks/livestream_dl) in your `PATH` for YouTube fallback downloads
- Go 1.21+ only if you build from source

Quick checks:

```powershell
yt-dlp --version
ffmpeg -version
node --version
npx -y twitch-dlp --help
```

## Get It Running

### 1. Download or build

From a release, download the archive for your OS from:

<https://github.com/Xanthopathy/streammon/releases>

Extract it, keep the config files next to the executable, then run:

```powershell
.\streammon.exe
```

On Linux or macOS:

```sh
./streammon
```

From source:

```powershell
.\scripts\build.ps1
```

or:

```powershell
go run .\cmd\streammon\main.go
```

### 2. Edit the configs

The app looks for these files beside the executable first, then in the current folder, then in `configs/`:

- `streammon_config.toml`
- `streammon_config_yt.toml`
- `streammon_config_twitch.toml`

The release zip includes example configs. Start by changing the channel lists.

YouTube channel example:

```toml
[[channel]]
id = "UCFzQd4pZ43ZNEdWBFe7QOKA" # UC...
name = "Saya Sairroxs"
filters = ["(?i).*karaoke.*|.*archive.*|.*guerilla.*|.*gorilla.*|.*gorila.*|.*surprise.*|.*handcam.*|.*asmr.*"]
```

Twitch channel example:

```toml
[[channel]]
id = "sayasairroxs"
name = "Saya Sairroxs"
filters = [".*"]
```

Leave `filters` empty or omit it to download every live stream for that channel.

Useful filter examples:

```toml
filters = ["(?i).*karaoke.*"]              # title contains karaoke
filters = ["(?i).*watchalong.*"]           # title contains watchalong
filters = ["(?i).*(live|birthday).*"]      # title contains live or birthday
filters = ["(?i)^.*(concert|3d).*"]        # title contains concert or 3d
```

`(?i)` makes the match case-insensitive.

### 3. Run it in a terminal

Running from a terminal is easier than double-clicking because you can see warnings and status messages.

```powershell
.\streammon.exe
```

By default, downloads are written to:

- `download_yt/`
- `download_twitch/`

Each channel gets its own folder. If download logs are enabled, each download also gets a `.log` file with the downloader command and subprocess output.

## Settings You Will Probably Touch

In `streammon_config.toml`:

| Setting                                                  | What it does                                                                                                       |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `timezone`                                               | Timestamp timezone for logs, like `"UTC"`, `"Asia/Tokyo"`, or `"UTC+7"`.                                           |
| `max_concurrent_downloads`                               | Total active downloads allowed across YouTube and Twitch.                                                          |
| `enable_youtube` / `enable_twitch`                       | Turn each platform on or off.                                                                                      |
| `save_download_logs`                                     | Save per-download `.log` files.                                                                                    |
| `clear_all_lockfiles`                                    | Remove old `.lock-*` files on startup. Helpful after crashes.                                                      |
| `youtube_archive_downloads` / `twitch_archive_downloads` | Write completed stream IDs to `youtube_archive.txt` / `twitch_archive.txt` so they are not downloaded again later. |
| `youtube_dlp_verbose_debug` / `twitch_dlp_verbose_debug` | Show raw downloader output in the terminal.                                                                        |
| `youtube_api_verbose_debug` / `twitch_api_verbose_debug` | Show detailed API/RSS checks. Usually leave off unless debugging.                                                  |

`timezone` accepts IANA names like `"UTC"`, `"America/New_York"`,
`"Europe/London"`, or `"Asia/Tokyo"`. Fixed offsets like `"UTC-5"` and
`"UTC+3"` also work.

`subprocess_progress_interval` and `subprocess_wait_interval` are in seconds.
Set progress interval to `0` to log every progress update, or increase it if
download logs are too noisy.

In `streammon_config_yt.toml`:

| Setting                   | What it does                                                               |
| ------------------------- | -------------------------------------------------------------------------- |
| `working_directory`       | Where YouTube files go.                                                    |
| `args`                    | Arguments passed to `yt-dlp`.                                              |
| `poll_interval`           | Delay between full channel-list checks.                                    |
| `check_method`            | `"rss"` or `"live"`. The other method is used as fallback.                 |
| `fallback_duration`       | How long YouTube sticks to the fallback method after it works.             |
| `ignore_older_than`       | Prevents older RSS entries from being treated as new live streams.         |
| `max_requests_per_second` | Safety limit for channel checks.                                           |
| `member_downloader`       | Downloader used for members-only streams: `"livestream_dl"` or `"yt-dlp"`. |
| `download_wait_retries` | Stop a stalled YouTube downloader after this many `[wait]` retry lines. |

`working_directory` can be relative to where you run streammon, or absolute.
Examples:

```toml
working_directory = "download_yt"
working_directory = "../downloads/youtube"
working_directory = "C:\\Archives\\YouTube"
working_directory = "/mnt/media/youtube"
```

`poll_interval` is the freshness target for a full channel-list check. streammon
spreads individual channel requests across that interval while also respecting
`max_requests_per_second`. For example, 40 channels at `poll_interval = "60s"`
spaces checks about 1.5 seconds apart, unless the RPS limit forces a slower
cycle. Lower intervals detect streams sooner but create more traffic.

For YouTube, be cautious with high request rates. If YouTube softblocks or
rate-limits checks, back off and give it time before trying again.

`check_method = "rss"` is lower bandwidth but can lag behind YouTube updates. Also likes to go down for a couple hours, but that happens during downtime (when americans are asleep).
`check_method = "live"` checks the channel `/live` page and is usually more
direct, but heavier. If the configured method fails, streammon tries the other
method and keeps using a working fallback for `fallback_duration`.

The optional `[livestream_dl]` block enables one `livestream_dl` retry after a
failed YouTube `yt-dlp` download. Leave it disabled unless `livestream_dl` is
installed and available in your `PATH`.

For account-required or members-only YouTube streams, follow
[yt-dlp's persistent-cookie instructions](https://github.com/yt-dlp/yt-dlp/wiki/extractors#exporting-youtube-cookies),
fill in `youtube_cookies.txt`, and keep that file private.

`youtube_cookies.txt` is used by streammon for members-only stream checks and
members-only downloads. When a members-only stream is found, streammon passes
`youtube_cookies.txt` to the configured `member_downloader`. The default is
`livestream_dl`, because YouTube downloads with yt-dlp cookies can stall
indefinitely, but you can set `member_downloader = "yt-dlp"` if you prefer.

streammon watches YouTube downloaders for repeated `[wait]` lines. After
`download_wait_retries` wait lines, it stops the downloader. If the stalled
process was `yt-dlp`, normal fallback handling can then try `livestream_dl`; for
member streams, that fallback uses `youtube_cookies.txt`. Set the value to `0`
to disable this stall guard.

`member_check_all = true` runs the members-only playlist check for every
configured YouTube channel. It is convenient, but heavier and noisier if you
track channels you are not membered to. The more precise setup is to leave it
false and set `member_check = true` only on specific `[[channel]]` entries.

In `streammon_config_twitch.toml`:

| Setting                   | What it does                            |
| ------------------------- | --------------------------------------- |
| `working_directory`       | Where Twitch files go.                  |
| `args`                    | Arguments passed to `twitch-dlp`.       |
| `poll_interval`           | Delay between full channel-list checks. |
| `max_requests_per_second` | Safety limit for GraphQL checks.        |

Twitch uses the same `working_directory`, `poll_interval`, and
`max_requests_per_second` ideas as YouTube. Twitch is generally more lenient
than YouTube, but keeping a reasonable request rate is still good practice.

## What The Logs Mean

- `is now LIVE`: the stream passed your filters and is eligible for download.
- `has gone offline`: the platform says the stream ended.
- `skipped: found in archive`: the stream ID is already in `archive.txt`.
- `already queued/downloading`: a `.lock-*` file exists for that stream.
- `Connection lost (confirmed)`: checks pause until the connection is stable again.
- `Config:` warnings: a config key is missing, invalid, or unknown. streammon tells you what default it used.
- `[Diagnostic]`: downloader exit details used to decide whether the file completed successfully.

## Duplicate Protection

streammon uses three layers:

- `archive.txt` remembers successful downloads across restarts.
- An in-memory session cache prevents re-queueing a stream during the current run.
- `.lock-*` files prevent multiple app instances from starting the same stream.

If the app says a stream is already queued after a crash, either leave `clear_all_lockfiles = true` and restart, or remove the named `.lock-*` file from the working directory.

## Practical Troubleshooting

If nothing downloads:

1. Run from a terminal and check for `Config:` warnings.
2. Confirm the platform is enabled in `streammon_config.toml`.
3. Temporarily remove `filters` from one channel to prove detection works.
4. Check that `yt-dlp`, `ffmpeg`, `node`, and `npx -y twitch-dlp --help` work.
5. Turn on the relevant API debug flag only while testing.

If YouTube misses streams:

- Try `check_method = "live"` for more direct live-page checks.
- Keep `fallback_duration` enabled so a working fallback stays active for a while.
- Make sure `ignore_older_than` is not too short for your use case.

If Twitch files finish but report oddly:

- Check the per-download `.log` file and the `[Diagnostic]` line.
- streammon considers Twitch successful when a media file exists and twitch-dlp either exits cleanly or emits a completion marker.

If your logs are too noisy:

- Set `youtube_dlp_verbose_debug = false` and `twitch_dlp_verbose_debug = false`.
- Increase `subprocess_progress_interval` and `subprocess_wait_interval`.
