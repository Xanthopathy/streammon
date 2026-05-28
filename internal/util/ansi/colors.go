package ansi

// ANSI color sequences used in terminal output.
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[91m"       // FATAL and YT
	ColorGreen  = "\033[92m"       // Live
	ColorYellow = "\033[93m"       // Timestamps and WARN
	ColorBlue   = "\033[94m"       // INFO, debug, lock
	ColorPurple = "\033[95m"       // Twitch
	ColorOrange = "\033[38;5;208m" // Channel Names
	ColorCyan   = "\033[96m"       // System
)
