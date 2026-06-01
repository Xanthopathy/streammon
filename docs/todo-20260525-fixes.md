# 2026-05-25 Fix Checklist

Use this checklist as the commit tracker for the eight items from `docs/todo.txt` lines 22-44. Each item should be implemented and committed separately.

- [x] 1. Clean successful YouTube merge residue files
  - Context: the `iaMNoyp7VGM` run left `.f140.mp4.part-Frag17112.part` and `.f299.mp4.part-Frag17112.part` files after yt-dlp reported a successful merge.
  - Include `docs/amanogawa_shiina-iaMNoyp7VGM.log` as debugging context.
  - Also clean saved log output so raw terminal erase sequences like `ESC[K[download]` do not remain in `.log` files.

- [x] 2. Retry when success is reported but the stream is still live
  - If diagnostics classify a downloader process as successful, wait for one full monitor loop.
  - If the same stream is still detected live after that loop, start another downloader instance for the same video instead of treating it as permanently done.

- [x] 3. Save complete downloader subprocess output to log files
  - `.log` files should receive every subprocess line regardless of `subprocess_progress_interval`.
  - Terminal output may continue to use progress/wait throttling.

- [skipped for now] 4. Add Docker support
  - Add a practical Docker setup for running streammon with mounted configs, archives, cookies, and download directories.

- [x] 5. Add cookie-based YouTube member stream checking
  - Copy the effective member-checking approach from holodownloader rather than relying on Holodex.
  - Use user-provided cookies for streams that require account access.

- [ ] 6. Add persistent YouTube cookie instructions
  - Document yt-dlp's persistent-cookie guidance:
    `https://github.com/yt-dlp/yt-dlp/wiki/extractors#exporting-youtube-cookies`
  - Put the user-facing template/instructions in the YouTube cookies example/template file.

- [ ] 7. Add optional `livestream_dl` fallback
  - Add `livestream_dl` as a fallback when standard yt-dlp livestream checks/downloads fail.
  - Add `livestream_dl`-specific args to `configs/streammon_config_yt.example.toml`.
  - Example shape:
    `livestream_dl --resolution best --threads 4 --segment-retries 10 --cookies ".\cookies.txt" --output "[%(upload_date)s] [%(id)s] [%(title)s] [%(channel)s].%(ext)s" --write-thumbnail --embed-thumbnail --wait-for-video "60" --ytdlp-command-line-options="--js-runtime node" J4niY0v3Ef4`

- [x] 8. Rework archive/cookie folder layout with migration
  - Current layout: root contains configs and executable; `download_yt/` and `download_twitch/` each contain their own `archive.txt`.
  - New layout: root contains `youtube_archive.txt`, `twitch_archive.txt`, and `youtube_cookies.txt`.
  - Add backward compatibility logic to move or merge old per-directory `archive.txt` files up one level.
