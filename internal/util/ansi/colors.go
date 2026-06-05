package ansi

// ANSI color sequences used in terminal output.
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[91m"       // [FATAL] and [YT]
	ColorGreen  = "\033[92m"       // Live and [SUCCESS]
	ColorYellow = "\033[93m"       // Timestamps and [WARN]
	ColorBlue   = "\033[94m"       // [yt-dlp] [twitch-dlp] [livestream_dl]
	ColorPurple = "\033[95m"       // [Twitch]
	ColorOrange = "\033[38;5;208m" // Channel names
	ColorCyan   = "\033[96m"       // [System]
	ColorTeal   = "\033[38;5;45m"  // Event tags ([LOCK], [DOWNLOAD], [ARCHIVE])
)
