package monitor

import (
	"path/filepath"
	"strings"
)

func downloaderNameFromCommand(path string, args []string) string {
	name := canonicalDownloaderName(path)
	if name == "npx" {
		for _, arg := range args[1:] {
			candidate := canonicalDownloaderName(arg)
			if candidate == "twitch-dlp" {
				return candidate
			}
		}
	}
	if name == "" {
		return "dlp"
	}
	return name
}

func canonicalDownloaderName(value string) string {
	name := strings.ToLower(filepath.Base(value))
	name = strings.TrimSuffix(name, ".exe")
	name = strings.TrimSuffix(name, ".cmd")
	name = strings.TrimSuffix(name, ".bat")

	switch name {
	case "yt-dlp", "livestream_dl", "twitch-dlp", "npx":
		return name
	default:
		return name
	}
}
