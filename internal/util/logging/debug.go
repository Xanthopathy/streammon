package logging

import "streammon/internal/util/ansi"

// DebugType identifies a configured debug output category.
type DebugType struct {
	name string
}

var (
	DebugTwitch     = DebugType{name: "Twitch"}
	DebugTwitchAPI  = DebugType{name: "TwitchAPI"}
	DebugTwitchDLP  = DebugType{name: "TwitchDLP"}
	DebugYouTube    = DebugType{name: "YouTube"}
	DebugYouTubeAPI = DebugType{name: "YouTubeAPI"}
	DebugYouTubeDLP = DebugType{name: "YouTubeDLP"}
)

func (d DebugType) String() string {
	return d.name
}

// Debug logs a message if the corresponding debug flag is enabled in GlobalConfig.
// debugType controls visibility through the matching debug config flag.
func (l *Logger) Debug(debugType DebugType, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	shouldLog := false

	// Determine Visibility based on specific flags
	switch debugType {
	case DebugTwitchAPI:
		shouldLog = l.globalCfg.TwitchAPIVerboseDebug
	case DebugTwitchDLP:
		shouldLog = l.globalCfg.TwitchDlpVerboseDebug
	case DebugYouTubeAPI:
		shouldLog = l.globalCfg.YoutubeAPIVerboseDebug
	case DebugYouTubeDLP:
		shouldLog = l.globalCfg.YoutubeDlpVerboseDebug
	case DebugTwitch:
		shouldLog = l.globalCfg.TwitchVerboseDebug
	case DebugYouTube:
		shouldLog = l.globalCfg.YoutubeVerboseDebug
	}

	if shouldLog {
		l.writeLine(l.formatTaggedLine(ansi.ColorBlue, debugType.String(), message), true)
	}
}
