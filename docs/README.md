# streammon

streammon watches YouTube and Twitch channels, checks when they go live, and starts [yt-dlp](https://github.com/yt-dlp/yt-dlp), [livestream_dl](https://github.com/CanOfSocks/livestream_dl), or [twitch-dlp](https://github.com/DmitryScaletta/twitch-dlp) for streams you actually want to save.

This is meant to replace the now deprecated [hoshinova](https://github.com/HoloArchivists/hoshinova). As of v1.1.0, `livestream_dl` is the default members-only YouTube downloader and can also be used as a fallback if a regular `yt-dlp` download fails.

## What You Need

- [FFmpeg](https://ffmpeg.org/) in your `PATH` for stream downloads.
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) in your `PATH` if you enable the YouTube monitor. streammon uses it for regular YouTube downloads and members-only playlist checks.
- [livestream_dl](https://github.com/CanOfSocks/livestream_dl) in your `PATH` if you want members-only YouTube downloads, enable the regular YouTube fallback path, or set it as the main YouTube downloader.
- [Node.js](https://nodejs.org/) and [twitch-dlp](https://github.com/DmitryScaletta/twitch-dlp) if you enable the Twitch monitor. Twitch downloads run through `npx twitch-dlp`.
- [Go 1.21+](https://go.dev/dl/) only if you build from source.

Quick checks:

Run the checks for the platform and downloader paths you enabled.

```powershell
yt-dlp --version
ffmpeg -version
node --version
npx -y twitch-dlp --help
livestream_dl --help
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

From source on any OS with Go installed, build or run directly:

```sh
go build -o streammon ./cmd/streammon
```

or:

```sh
go run ./cmd/streammon
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
member_check = false
```

To find a YouTube channel ID, use `yt-dlp` against the channel handle or URL.
streammon wants the `UC...` channel ID, not the `@handle`.

```powershell
yt-dlp --flat-playlist --playlist-items 1 --print "%(channel_id)s" "https://www.youtube.com/@SayaSairroxs"
```

Twitch channel example:

```toml
[[channel]]
id = "sayasairroxs"
name = "Saya Sairroxs"
filters = [".*"]
```

For Twitch, `id` is the channel login from the URL, such as `sayasairroxs` from
`https://www.twitch.tv/sayasairroxs`.

Leave `filters` empty or omit it to download every live stream for that channel.

Useful filter examples:

```toml
filters = ["(?i).*karaoke.*"]              # title contains karaoke
filters = ["(?i).*watchalong.*"]           # title contains watchalong
filters = ["(?i).*(live|birthday).*"]      # title contains live or birthday
filters = ["(?i)^.*(concert|3d).*"]        # title contains concert or 3d
```

Filters use Go regular expressions. See the [Go regexp syntax](https://pkg.go.dev/regexp/syntax) for the full pattern language. `(?i)` makes the match case-insensitive.

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

`subprocess_progress_interval` and `subprocess_wait_interval` are in seconds and
apply to both `.log` files and terminal subprocess output. Progress throttling
covers yt-dlp `[download]` lines and livestream_dl stats lines. Set progress
interval to `0` to log every progress update, or increase it if download logs
are too noisy.

In `streammon_config_yt.toml`:

| Setting                   | What it does                                                            |
| ------------------------- | ----------------------------------------------------------------------- |
| `working_directory`       | Where YouTube files go.                                                 |
| `[yt-dlp].args`           | Arguments passed to `yt-dlp`.                                           |
| `[livestream_dl].args`    | Arguments passed to `livestream_dl`.                                    |
| `poll_interval`           | Delay between full channel-list checks.                                 |
| `check_method`            | `"rss"` or `"live"`. The other method is used as fallback.              |
| `downloader_method`       | Regular-stream downloader: `"yt-dlp"` or `"livestream_dl"`.             |
| `fallback_duration`       | How long YouTube sticks to the fallback method after it works.          |
| `ignore_older_than`       | Prevents older RSS entries from being treated as new live streams.      |
| `max_requests_per_second` | Safety limit for channel checks.                                        |
| `member_downloader`       | Downloader used for members-only streams. Default: `"livestream_dl"`.   |
| `download_wait_retries`   | Stop a stalled YouTube downloader after this many `[wait]` retry lines. |
| `retry_same_downloader_with_timestamp_when_live` | Retry an early live completion with a timestamped output if no alternate downloader is available. |
| `retry_offline_without_live_args` | After an early completion, retry the final VOD with yt-dlp live-wait args removed once the stream is offline. |

`working_directory` can be relative to where you run streammon, or absolute.
Examples:

```toml
working_directory = "download_yt"
working_directory = "../downloads/youtube"
working_directory = "C:\\Archives\\YouTube"
working_directory = "/mnt/media/youtube"
```

Older configs that still put downloader `args` under `[streammon]` continue to
work for now, but streammon will warn and prefer `[yt-dlp].args` or
`[twitch-dlp].args`.

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

`downloader_method` controls the primary downloader for regular public YouTube
streams. The default is `yt-dlp`; set it to `livestream_dl` if you want
`livestream_dl` to be the main downloader instead. If the primary downloader
fails, streammon can try the other downloader as a one-shot fallback.

The `[livestream_dl]` block has two jobs. Its `args` are used when
`downloader_method = "livestream_dl"` downloads a regular stream and when
`member_downloader = "livestream_dl"` downloads a members-only stream. Its
`enabled` flag controls only the optional fallback from a regular public
`yt-dlp` download to `livestream_dl`. Unlike the `yt-dlp` output template,
the default `livestream_dl` template omits `.(ext)s` because `livestream_dl`
adds the final media extension itself.

For account-required or members-only YouTube streams, follow
[yt-dlp's persistent-cookie instructions](https://github.com/yt-dlp/yt-dlp/wiki/extractors#exporting-youtube-cookies),
fill in `youtube_cookies.txt`, and keep that file private.

`youtube_cookies.txt` is used by streammon for members-only stream checks and
members-only downloads. When a members-only stream is found, streammon passes
`youtube_cookies.txt` to the configured `member_downloader`. Both `yt-dlp` and
`livestream_dl` work for regular public streams, but members-only streams are
different: `yt-dlp` with YouTube cookies currently tends to stall there, while
`livestream_dl` can handle that path. That is why `member_downloader` defaults
to `livestream_dl`.

streammon watches YouTube downloaders for repeated `[wait]` lines. After
`download_wait_retries` wait lines, it stops the downloader. If the stalled
process was `yt-dlp`, normal fallback handling can then try `livestream_dl`; for
member streams, that fallback uses `youtube_cookies.txt`. Set the value to `0`
to disable this stall guard.

If a YouTube downloader completes while the same video is still live, streammon
normally retries with the alternate downloader when one is enabled. If only one
downloader is available, `retry_same_downloader_with_timestamp_when_live = true`
lets streammon run that same downloader again with a timestamped output name.
This can preserve more live content, but it can also create duplicate or split
files.

`retry_offline_without_live_args = true` adds a second recovery path only after
streammon has already confirmed that a completed downloader ended while the same
video was still live. When that pending result later resolves offline, streammon
runs a final yt-dlp VOD retry after removing `--live-from-start` and
`--wait-for-video`. This retry also uses a timestamped output name so yt-dlp
does not skip the existing early-merged file.

`member_check_all = true` runs the members-only playlist check for every
configured YouTube channel. It is convenient, but heavier and noisier if you
track channels you are not membered to. The more precise setup is to leave it
false and set `member_check = true` only on specific `[[channel]]` entries.

In `streammon_config_twitch.toml`:

| Setting                   | What it does                            |
| ------------------------- | --------------------------------------- |
| `working_directory`       | Where Twitch files go.                  |
| `[twitch-dlp].args`       | Arguments passed to `twitch-dlp`.       |
| `poll_interval`           | Delay between full channel-list checks. |
| `max_requests_per_second` | Safety limit for GraphQL checks.        |

Twitch uses the same `working_directory`, `poll_interval`, and
`max_requests_per_second` ideas as YouTube. Twitch is generally more lenient
than YouTube, but keeping a reasonable request rate is still good practice.

## What The Logs Mean

- `is now LIVE`: the stream passed your filters and is eligible for download.
- `has gone offline`: the platform says the stream ended.
- `skipped: found in archive`: the stream ID is already in `youtube_archive.txt` or `twitch_archive.txt`.
- `already queued/downloading`: a `.lock-*` file exists for that stream.
- `Connection lost (confirmed)`: checks pause until the connection is stable again.
- `Config:` warnings: a config key is missing, invalid, or unknown. streammon tells you what default it used.
- `[Diagnostic]`: downloader exit details used to decide whether the file completed successfully.

## Duplicate Protection

streammon uses three layers:

- `youtube_archive.txt` and `twitch_archive.txt` remember successful downloads across restarts.
- An in-memory session cache prevents re-queueing a stream during the current run.
- `.lock-*` files prevent multiple app instances from starting the same stream.

If the app says a stream is already queued after a crash, either leave `clear_all_lockfiles = true` and restart, or remove the named `.lock-*` file from the working directory.

## Practical Troubleshooting

If nothing downloads:

1. Run from a terminal and check for `Config:` warnings.
2. Confirm the platform is enabled in `streammon_config.toml`.
3. Temporarily remove `filters` from one channel to prove detection works.
4. Check the tools for the enabled platform: `yt-dlp` / `livestream_dl` for YouTube, or `node` and `npx -y twitch-dlp --help` for Twitch.
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

## Maintainer Notes

`scripts/build.ps1` is for publishing releases, not normal local builds. It creates a `build/` folder with Windows, Linux, and macOS release folders, copies the example configs into each folder with their runtime filenames, and writes zip archives for each target.

```powershell
.\scripts\build.ps1 -Clean
```
