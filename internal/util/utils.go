package util

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// --- Colors ---
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[91m"       // FATAL and YT
	ColorGreen  = "\033[92m"       // Live
	ColorYellow = "\033[93m"       // Timestamps and WARN
	ColorBlue   = "\033[94m"       // INFO, debug, lock
	ColorPurple = "\033[95m"       // Twitch
	ColorOrange = "\033[38;5;208m" // Channel Names
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
// Supports IANA timezone names (e.g., "Asia/Tokyo") or UTC offset format (e.g., "UTC+7", "UTC-5", "+7", "-5")
func FormatTime(t time.Time, timezone string) string {
	var loc *time.Location

	if timezone == "" {
		loc = time.UTC
	} else {
		// Try to load as IANA timezone first
		var err error
		loc, err = time.LoadLocation(timezone)

		if err != nil {
			// If that fails, try to parse as UTC offset format
			offsetStr := timezone
			// Handle "UTC+7" by removing "UTC" prefix
			if after, ok := strings.CutPrefix(offsetStr, "UTC"); ok {
				offsetStr = after
			}

			// Try to parse the offset string (e.g., "+7", "-5", "+5:30")
			offsetSeconds, parseErr := parseUTCOffset(offsetStr)
			if parseErr == nil {
				loc = time.FixedZone("UTC", offsetSeconds)
			} else {
				// Fall back to UTC if offset parsing fails
				loc = time.UTC
			}
		}
	}

	// The format "MST-07:00" includes the timezone name and offset, e.g., "UTC+00:00".
	formattedTime := t.In(loc).Format("2006-01-02 15:04:05 MST-07:00")
	return fmt.Sprintf("[%s%s%s]", ColorYellow, formattedTime, ColorReset)
}

// parseUTCOffset parses UTC offset strings like "+7", "-5", "+5:30" and returns offset in seconds
func parseUTCOffset(offsetStr string) (int, error) {
	offsetStr = strings.TrimSpace(offsetStr)

	if offsetStr == "" {
		// Empty offset means UTC
		return 0, nil
	}

	// Determine sign
	var sign int64 = 1
	if strings.HasPrefix(offsetStr, "-") {
		sign = -1
		offsetStr = offsetStr[1:]
	} else if strings.HasPrefix(offsetStr, "+") {
		offsetStr = offsetStr[1:]
	}

	// Parse hours and optional minutes
	parts := strings.Split(offsetStr, ":")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid offset format")
	}

	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	var minutes int64 = 0
	if len(parts) == 2 {
		minutes, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, err
		}
	}

	totalSeconds := sign * (hours*3600 + minutes*60)
	return int(totalSeconds), nil
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

// JoinCommandArgs joins command arguments into a single string, quoting args that contain spaces
func JoinCommandArgs(args []string) string {
	var result []string
	for _, arg := range args {
		if strings.Contains(arg, " ") {
			result = append(result, fmt.Sprintf(`"%s"`, arg))
		} else {
			result = append(result, arg)
		}
	}
	return strings.Join(result, " ")
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
	err := os.Remove(path)
	_ = err // Ignore error, best-effort deletion
}

// --- File Helpers ---

func AppendLineToFile(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}
	return nil
}

// ReadLinesToSet reads a file and returns a map of non-empty lines.
// Used for loading archive.txt into memory.
func ReadLinesToSet(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	lines := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			lines[text] = true
		}
	}
	return lines, scanner.Err()
}
