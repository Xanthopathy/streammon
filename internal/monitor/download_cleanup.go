package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"streammon/internal/util/logging"
	"strings"
)

func isYTDLPResidueFile(name string, proc *downloadProcess) bool {
	if proc.downloaderName != "yt-dlp" {
		return false
	}
	if !strings.Contains(name, proc.videoID) {
		return false
	}

	return strings.Contains(name, ".part-Frag") || strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".temp")
}

func cleanupYTDLPResidue(dir string, proc *downloadProcess, logger *logging.Logger) {
	if dir == "" || proc.downloaderName != "yt-dlp" {
		return
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn(fmt.Sprintf("Could not scan for yt-dlp residue files: %v", err))
		return
	}

	removed := 0
	for _, file := range files {
		if file.IsDir() || !isYTDLPResidueFile(file.Name(), proc) {
			continue
		}

		path := filepath.Join(dir, file.Name())
		if err := os.Remove(path); err != nil {
			logger.Warn(fmt.Sprintf("Could not remove yt-dlp residue file %s: %v", file.Name(), err))
			continue
		}
		removed++
	}

	if removed > 0 {
		logger.LogEventf("CLEANUP", "Cleaned up %d yt-dlp residue file(s).", removed)
	}
}
