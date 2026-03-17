package util

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"streammon/internal/config"
)

// LogLevel defines the logging severity level
type LogLevel int

const (
	LogLevelRegular LogLevel = iota // Regular logs only
	LogLevelDebug                   // All logs including debug spam
)

// DownloadLogger handles logging for a download session
// Single .log file for all output (subprocess, events, errors)
// Terminal output controlled by debug toggles
type DownloadLogger struct {
	mu                    sync.Mutex
	logFile               *os.File
	globalCfg             *config.GlobalConfig
	logPrefix             string
	logColor              string
	channelID             string
	channelName           string
	streamID              string
	lastDownloadWriteTime time.Time
	lastWaitWriteTime     time.Time
	// Debug flags for what to show in terminal
	apiDebug bool // For TwitchAPI/YouTube API calls
	dlpDebug bool // For twitch-dlp/yt-dlp subprocess output
}

// NewDownloadLogger creates a new logger for a download session
// logPrefix: "YT" or "Twitch"
// logColor: use ColorRed, ColorPurple, etc.
// apiDebug: show API calls in terminal
// dlpDebug: show subprocess output in terminal
// command: the subprocess command string to write at the top of the log file
// logFile will be created only if save_download_logs is true in globalCfg
func NewDownloadLogger(
	channelDir string,
	channelID string,
	channelName string,
	streamID string,
	dateCreated time.Time,
	globalCfg *config.GlobalConfig,
	logPrefix string,
	logColor string,
	apiDebug bool,
	dlpDebug bool,
	command string,
) (*DownloadLogger, error) {
	sanitizedName := SanitizeFolderName(channelName)
	dateStr := dateCreated.UTC().Format("2006-01-02")
	baseFilename := fmt.Sprintf("%s-%s-%s", dateStr, sanitizedName, streamID)

	logger := &DownloadLogger{
		globalCfg:   globalCfg,
		logPrefix:   logPrefix,
		logColor:    logColor,
		channelID:   channelID,
		channelName: channelName,
		streamID:    streamID,
		apiDebug:    apiDebug,
		dlpDebug:    dlpDebug,
	}

	// Create single log file only if save_download_logs is enabled
	if globalCfg.SaveDownloadLogs {
		logPath := filepath.Join(channelDir, baseFilename+".log")
		if file, err := os.Create(logPath); err == nil {
			logger.logFile = file
			// Write the command at the top of the log file for reference
			if command != "" {
				file.WriteString("=== Subprocess Command ===\n")
				file.WriteString(command + "\n")
				file.WriteString("=======================\n\n")
				file.Sync()
			}
		} else {
			fmt.Printf("%s [%s%s%s] Warning: Failed to create log file: %v\n",
				FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, ColorReset, err)
		}
	}

	return logger, nil
}

// stripANSI removes ANSI color codes from a string
// Used to remove color codes before writing to log files
func stripANSI(s string) string {
	// Regex pattern to match ANSI escape sequences
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiPattern.ReplaceAllString(s, "")
}

// LogRegular logs a message to both terminal and log file
// These are important events that should always be visible
func (l *DownloadLogger) LogRegular(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := FormatTime(time.Now(), l.globalCfg.Timezone)
	line := fmt.Sprintf("%s [%s%s%s] %s\n", timestamp, l.logColor, l.logPrefix, ColorReset, message)

	// Always show important events in terminal
	fmt.Print(line)

	// Always write to log file without color codes
	if l.logFile != nil {
		cleanedLine := stripANSI(line)
		l.logFile.WriteString(cleanedLine)
		l.logFile.Sync()
	}
}

// LogDebug logs a message only if debug is enabled
// Used for API calls and verbose diagnostics
func (l *DownloadLogger) LogDebug(message string) {
	if !l.apiDebug {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := FormatTime(time.Now(), l.globalCfg.Timezone)
	line := fmt.Sprintf("%s [%s%s%s][DEBUG] %s\n", timestamp, l.logColor, l.logPrefix, ColorReset, message)

	// Terminal output if debug enabled
	fmt.Print(line)

	// Also write to log file without color codes
	if l.logFile != nil {
		cleanedLine := stripANSI(line)
		l.logFile.WriteString(cleanedLine)
		l.logFile.Sync()
	}
}

// LogSubprocessOutput writes subprocess output (from yt-dlp/twitch-dlp)
// Terminal visibility controlled by dlpDebug flag
// Progress lines are throttled based on subprocess_progress_interval config
// Log file always receives subprocess output (with throttling for progress lines)
// debugType: the specific subprocess type (e.g., "yt-dlp", "twitch-dlp")
func (l *DownloadLogger) LogSubprocessOutput(output string, debugType string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := FormatTime(time.Now(), l.globalCfg.Timezone)
	// Format: [time] [Platform] [debugType] [channelName] message
	// Platform in its color, debugType in blue
	line := fmt.Sprintf("%s [%s%s%s] [%s%s%s] [%s] %s\n", timestamp, l.logColor, l.logPrefix, ColorReset, ColorBlue, debugType, ColorReset, l.channelName, output)

	// Check if this is a progress line (contains [download] or [wait])
	isDownloadLine := strings.Contains(output, "[download]")
	isWaitLine := strings.Contains(output, "[wait]")

	// Determine if we should write based on throttling
	shouldWrite := true

	if isDownloadLine && l.globalCfg.SubprocessProgressInterval > 0 {
		// Apply throttling for [download] lines
		now := time.Now()
		if !l.lastDownloadWriteTime.IsZero() && now.Sub(l.lastDownloadWriteTime) < time.Duration(l.globalCfg.SubprocessProgressInterval)*time.Second {
			shouldWrite = false
		}
		if shouldWrite {
			l.lastDownloadWriteTime = now
		}
	} else if isWaitLine && l.globalCfg.SubprocessWaitInterval > 0 {
		// Apply throttling for [wait] lines
		now := time.Now()
		if !l.lastWaitWriteTime.IsZero() && now.Sub(l.lastWaitWriteTime) < time.Duration(l.globalCfg.SubprocessWaitInterval)*time.Second {
			shouldWrite = false
		}
		if shouldWrite {
			l.lastWaitWriteTime = now
		}
	}

	// Show in terminal only if dlp debug is enabled and throttling allows it
	if shouldWrite && l.dlpDebug {
		fmt.Print(line)
	}

	// Always write to log file (with throttling applied)
	if shouldWrite && l.logFile != nil {
		cleanedLine := stripANSI(line)
		l.logFile.WriteString(cleanedLine)
		l.logFile.Sync()
	}
}

// LogProgress writes download progress (always shown in terminal and logged)
func (l *DownloadLogger) LogProgress(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := FormatTime(time.Now(), l.globalCfg.Timezone)
	line := fmt.Sprintf("%s [%s%s%s] [PROGRESS] %s\n", timestamp, l.logColor, l.logPrefix, ColorReset, message)

	// Terminal output
	fmt.Print(line)

	// Log file without color codes
	if l.logFile != nil {
		cleanedLine := stripANSI(line)
		l.logFile.WriteString(cleanedLine)
		l.logFile.Sync()
	}
}

// LogError logs an error to both terminal and log file
func (l *DownloadLogger) LogError(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := FormatTime(time.Now(), l.globalCfg.Timezone)
	line := fmt.Sprintf("%s [%s%s%s] %sERROR%s: %s\n", timestamp, l.logColor, l.logPrefix, ColorReset, ColorRed, ColorReset, message)

	// Terminal output
	fmt.Print(line)

	// Log file without color codes
	if l.logFile != nil {
		cleanedLine := stripANSI(line)
		l.logFile.WriteString(cleanedLine)
		l.logFile.Sync()
	}
}

// Close flushes and closes the log file
func (l *DownloadLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logFile != nil {
		l.logFile.Close()
	}
}

// scanCRLF is a custom split function for bufio.Scanner that recognizes
// \n, \r, and \r\n as line delimiters. This is needed for progress output
// from twitch-dlp and yt-dlp which use \r to overwrite lines in the terminal.
func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			token := data[:i]
			advance := i + 1

			// Handle \r\n as a single delimiter
			if i+1 < len(data) && data[i] == '\r' && data[i+1] == '\n' {
				advance = i + 2
			}

			return advance, token, nil
		}
	}

	// At EOF, return remaining data as token if not empty
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}

	return 0, nil, nil
}

// ReadPipeAndLog reads from a pipe and logs each line as subprocess output
// Used for capturing twitch-dlp/yt-dlp stdout/stderr
// Handles \r, \n, and \r\n line endings to capture progress output
// debugType: the specific subprocess type (e.g., "YT-DLP", "TWITCH-DLP")
func ReadPipeAndLog(pipe io.Reader, logger *DownloadLogger, debugType string) {
	scanner := bufio.NewScanner(pipe)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		text := scanner.Text()
		// Skip empty lines (can happen with consecutive \r characters)
		if len(text) > 0 {
			logger.LogSubprocessOutput(text, debugType)
		}
	}
}
