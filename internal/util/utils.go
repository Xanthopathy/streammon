package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"streammon/internal/config"
)

// --- Colors ---
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[91m"
	ColorGreen  = "\033[92m"
	ColorYellow = "\033[93m"
	ColorBlue   = "\033[94m"
	ColorCyan   = "\033[96m"
	ColorPurple = "\033[95m"
)

// --- UI Helpers ---

// SetTerminalTitle sets the terminal window title (cross-platform).
func SetTerminalTitle(title string) {
	if runtime.GOOS == "windows" {
		// Windows: Use title command
		cmd := exec.Command("cmd", "/c", "title", title)
		cmd.Run()
	} else {
		// Linux/macOS: Use ANSI escape sequence
		fmt.Printf("\x1b]0;%s\x07", title)
	}
}

func PrintBanner() {
	// Clear console
	cmd := exec.Command("cmd", "/c", "cls")
	if runtime.GOOS != "windows" {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Println(ColorBlue + strings.Repeat("=", 60) + ColorReset)
	fmt.Printf(" %sStreamMon%s - Automated Stream Archiver\n", ColorGreen, ColorReset)
	fmt.Println(ColorBlue + strings.Repeat("=", 60) + ColorReset)
}

// --- Time Helpers ---

// FormatTime formats a time.Time object into the standard log timestamp string.
// It respects the timezone string provided, defaulting to UTC if invalid/empty.
func FormatTime(t time.Time, timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc = time.UTC
	}
	// The format "MST-07:00" includes the timezone name and offset, e.g., "UTC+00:00".
	formattedTime := t.In(loc).Format("2006-01-02 15:04:05 MST-07:00")
	return fmt.Sprintf("[%s%s%s]", ColorYellow, formattedTime, ColorReset) // White borders [], yellow text
}

// --- Logging Helpers ---

func DebugLog(cfg *config.GlobalConfig, module, message string) {
	var shouldLog bool
	if strings.HasPrefix(module, "Twitch") && cfg.TwitchVerboseDebug {
		shouldLog = true
	}
	if strings.HasPrefix(module, "YouTube") && cfg.YoutubeVerboseDebug {
		shouldLog = true
	}

	if shouldLog {
		fmt.Printf("%s [%sDEBUG%s][%s] %s\n", FormatTime(time.Now(), cfg.Timezone), ColorCyan, ColorReset, module, message)
	}
}

// --- String Helpers ---

// SanitizeFilename replaces invalid characters with underscores (matches python logic)
func SanitizeFilename(name string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	return re.ReplaceAllString(name, "_")
}

// SanitizeFolderName converts to lowercase, replaces spaces with _, and removes invalid chars
func SanitizeFolderName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	return SanitizeFilename(name)
}

// --- Lockfile Helpers ---

func GetLockfilePath(workDir, channelName, id string) string {
	sanitizedName := SanitizeFolderName(channelName)
	filename := fmt.Sprintf(".lock-%s-%s", sanitizedName, id)
	return filepath.Join(workDir, filename)
}

func HasLock(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func CreateLock(path string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory %s: %w", dir, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func DeleteLock(path string) {
	os.Remove(path)
}
