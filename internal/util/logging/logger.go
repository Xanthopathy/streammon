package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/util/ansi"
	"streammon/internal/util/text"
	"streammon/internal/util/timefmt"
)

// Logger handles logging for a monitor or a download session.
// Single .log file for all output (subprocess, events, errors)
// Terminal output controlled by debug toggles
type Logger struct {
	mu                    sync.Mutex
	logFile               *os.File // Optional: for download-specific logs
	globalCfg             *config.GlobalConfig
	logPrefix             string
	logColor              string
	channelName           string
	lastDownloadWriteTime time.Time
	lastWaitWriteTime     time.Time
	// Debug flags for what to show in terminal
	dlpDebug bool // For twitch-dlp/yt-dlp subprocess output
}

// NewLogger creates a new logger for general monitor-level logging (terminal only).
func NewLogger(globalCfg *config.GlobalConfig, logPrefix, logColor string) *Logger {
	return &Logger{
		globalCfg:   globalCfg,
		logPrefix:   logPrefix,
		logColor:    logColor,
		channelName: "Monitor", // Default context
	}
}

// NewLoggerForDownload creates a new logger for a specific download session.
// It can write to a dedicated log file in addition to the terminal.
// logPrefix: "YT" or "Twitch"
// logColor: use ColorRed, ColorPurple, etc.
// dlpDebug: show subprocess output in terminal
// command: the subprocess command string to write at the top of the log file
// logFile will be created only if save_download_logs is true in globalCfg
func NewLoggerForDownload(
	channelDir string,
	channelName string,
	streamID string,
	globalCfg *config.GlobalConfig,
	logPrefix string,
	logColor string,
	dlpDebug bool,
	command string,
) (*Logger, error) {
	sanitizedName := text.SanitizeFolderName(channelName)
	baseFilename := fmt.Sprintf("%s-%s", sanitizedName, streamID)

	logger := &Logger{
		globalCfg:   globalCfg,
		logPrefix:   logPrefix,
		logColor:    logColor,
		channelName: channelName,
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
			fmt.Printf("%s Warning: Failed to create log file: %v\n",
				formatLogPrefix(globalCfg, logPrefix, logColor), err)
		}
	}

	return logger, nil
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// stripANSI removes ANSI/CSI terminal control sequences before writing log files.
func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

// formatLogPrefix returns the shared timestamp/platform prefix for log lines.
func formatLogPrefix(globalCfg *config.GlobalConfig, logPrefix, logColor string) string {
	return fmt.Sprintf("%s [%s%s%s]", timefmt.FormatTime(time.Now(), globalCfg.Timezone), logColor, logPrefix, ansi.ColorReset)
}

func (l *Logger) linePrefix() string {
	return formatLogPrefix(l.globalCfg, l.logPrefix, l.logColor)
}

func (l *Logger) formatLine(message string) string {
	return fmt.Sprintf("%s %s\n", l.linePrefix(), message)
}

func (l *Logger) formatTaggedLine(tagColor, tag, message string) string {
	return fmt.Sprintf("%s [%s%s%s] %s\n", l.linePrefix(), tagColor, tag, ansi.ColorReset, message)
}

func (l *Logger) taggedPrefix(tagColor, tag string) string {
	return fmt.Sprintf("%s [%s%s%s]", l.linePrefix(), tagColor, tag, ansi.ColorReset)
}

func (l *Logger) writeFileLine(line string) {
	if l.logFile == nil {
		return
	}

	cleanedLine := stripANSI(line)
	l.logFile.WriteString(cleanedLine)
	l.logFile.Sync()
	// Sync() after every subprocess line is durable but potentially expensive now that logs are complete. If disk activity becomes noisy during long streams, optimize buffering or sync frequency.
}

func (l *Logger) writeLine(line string, terminal bool) {
	if terminal {
		fmt.Print(line)
	}
	l.writeFileLine(line)
}

// LogRegular logs a message to both terminal and log file
// These are important events that should always be visible
func (l *Logger) LogRegular(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.writeLine(l.formatLine(message), true)
}

// Logf is a convenience wrapper for LogRegular that uses fmt.Sprintf.
func (l *Logger) Logf(format string, args ...any) {
	l.LogRegular(fmt.Sprintf(format, args...))
}

// LogEvent logs a message with a specific event type tag (e.g., "LOCK").
func (l *Logger) LogEvent(eventType, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.writeLine(l.formatTaggedLine(ansi.ColorTeal, eventType, message), true)
}

// LogEventf is a convenience wrapper for LogEvent that uses fmt.Sprintf.
func (l *Logger) LogEventf(eventType, format string, args ...any) {
	l.LogEvent(eventType, fmt.Sprintf(format, args...))
}

// LogError logs an error to both terminal and log file
func (l *Logger) LogError(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.writeLine(l.formatLine(fmt.Sprintf("%sERROR%s: %s", ansi.ColorRed, ansi.ColorReset, message)), true)
}

// LogErrorf is a convenience wrapper for LogError that uses fmt.Sprintf.
func (l *Logger) LogErrorf(format string, args ...any) {
	l.LogError(fmt.Sprintf(format, args...))
}

// Warn logs a warning message.
func (l *Logger) Warn(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.writeLine(l.formatTaggedLine(ansi.ColorYellow, "WARN", message), true)
}

// Close flushes and closes the log file
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logFile != nil {
		l.logFile.Close()
	}
}
