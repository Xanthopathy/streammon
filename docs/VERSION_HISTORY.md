# StreamMon - Version History

This release recap was reconstructed from the git tag and commit history.

## v1.0.0 - 2026-03-17

Initial tagged release. Established the Twitch and YouTube monitoring foundation, global configuration, timezone-aware terminal logging, download subprocess logging, lockfile handling, per-channel download folders, build/release scripts, config filename cleanup, and the first process/readme documentation pass.

## v1.0.1 - 2026-03-17

Fixed config capitalization and runtime config filenames, added session tracking for already-downloaded videos to avoid duplicate log spam, and merged the main branch state into the tagged release.

## v1.0.2 - 2026-03-18

Added persistent archive files for downloaded stream IDs so completed streams are not re-downloaded, stopped Twitch download subprocesses when streams go offline, and fixed folder creation for non-Windows builds.

## v1.0.3 - 2026-03-18

Made channel monitoring more resilient with error backoff and jitter, and added a YouTube request User-Agent to reduce bot-flagging risk.

## v1.0.4 - 2026-03-21

Reworked polling around fixed-delay scheduling, poll jitter, safer interval defaults, and `max_requests_per_second` request spacing. Also improved logging consistency, download slot and lock logs, intentional termination success handling, startup lockfile cleanup, and request-rate safety warnings.

## v1.0.5 - 2026-03-21

Added `/live` page checking as an alternate YouTube detection method, introduced internet connectivity monitoring and pause/resume handling, added version handling, and polished related terminal coloring and logging behavior.

## v1.0.6 - 2026-03-22

Fixed config file search behavior, improved YouTube fallback handling and method stats reporting, updated version checking, split YouTube/Twitch scraper code into focused packages, moved docs and scripts into their own directories, added example config handling in the build script, and cleaned up older monitor structure.

## v1.0.7 - 2026-04-19

Overhauled network checking, separated network errors from subprocess errors, reduced error backoff ramping, strengthened download success verification and diagnostics, and added a short post-exit grace period so downloader cleanup can finish before streammon evaluates results.

## v1.0.8 - 2026-04-20

Widened network-error detection, fixed a connection-monitor broadcast deadlock during longer outages, and made `[Diagnostic]` log tags blue.

## v1.0.9 - 2026-05-28

Added config validation warnings, startup lockfile cleanup, safer YouTube fallback-state locking, fixed request-spacing math for fractional RPS values, logged return to normal poll intervals after errors clear, separated Twitch success diagnostics from YouTube merger checks, improved downloader wait/offline handling, and refactored utility packages without changing the external workflow.

## v1.1.0 - 2026-06-03

Added root-level archive files with legacy migration, YouTube members-only discovery through cookie-backed playlist checks, scoped cookie use for member downloads, `livestream_dl` as the default members-only downloader plus optional public-stream fallback, stalled `[wait]` retry termination, safer pending-success handling for long YouTube streams, yt-dlp residue cleanup, fuller subprocess logs, and broader package splits for config, monitoring, download lifecycle, logging, UTC offsets, and lockfiles.

## v1.1.1 - 2026-06-04

Fixed Windows downloader-name normalization, avoided repeating a completed YouTube downloader in recovery paths, detected `livestream_dl` completion markers more reliably, tightened early YouTube completion retries, added YouTube early-completion recovery config options, documented the recovery behavior, and throttled `livestream_dl` progress output.

## v1.1.2 - 2026-06-05

Shortened log timestamp timezone formatting, colorized `livestream_dl` warning/progress output, and added explicit tags for streammon lifecycle logs.
