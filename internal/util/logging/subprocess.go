package logging

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"streammon/internal/util/ansi"
)

func (l *Logger) formatSubprocessLine(debugType, output string) string {
	return fmt.Sprintf("%s [%s%s%s] %s\n",
		l.taggedPrefix(ansi.ColorBlue, debugType),
		ansi.ColorOrange, l.channelName, ansi.ColorReset,
		output)
}

// LogSubprocessOutput writes subprocess output (from yt-dlp/twitch-dlp)
// Log files receive every subprocess line.
// Terminal visibility is controlled by dlpDebug and progress throttling.
// debugType: the specific subprocess type (e.g., "yt-dlp", "twitch-dlp")
func (l *Logger) LogSubprocessOutput(output string, debugType string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Format: [time] [Platform] [debugType] [ChannelName] message
	line := l.formatSubprocessLine(debugType, output)

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

	// Preserve every subprocess line in the download log
	l.writeFileLine(line)

	// Throttle terminal output independently from file logging
	if shouldWrite && l.dlpDebug {
		fmt.Print(line)
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
func ReadPipeAndLog(pipe io.Reader, logger *Logger, debugType string, callback func(string)) {
	scanner := bufio.NewScanner(pipe)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		text := scanner.Text()
		// Skip empty lines (can happen with consecutive \r characters)
		if len(text) > 0 {
			logger.LogSubprocessOutput(text, debugType)
			if callback != nil {
				callback(text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logger.LogError(fmt.Sprintf("Error reading subprocess output: %v", err))
	}
}
