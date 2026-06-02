package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"streammon/internal/util/fileio"
	"streammon/internal/util/logging"
)

func (b *BaseMonitor) archiveFilename() string {
	switch b.controller.GetLogPrefix() {
	case logPrefixYouTube:
		return "youtube_archive.txt"
	case logPrefixTwitch:
		return "twitch_archive.txt"
	default:
		return "archive.txt"
	}
}

func (b *BaseMonitor) archivePath() string {
	return b.archiveFilename()
}

func (b *BaseMonitor) legacyArchivePath() string {
	return filepath.Join(
		b.controller.GetStreamMonConfig().WorkingDirectory,
		"archive.txt",
	)
}

func (b *BaseMonitor) migrateLegacyArchive(logger *logging.Logger) {
	newPath := b.archivePath()
	oldPath := b.legacyArchivePath()

	_, newErr := os.Stat(newPath)
	_, oldErr := os.Stat(oldPath)

	newExists := newErr == nil
	oldExists := oldErr == nil

	if !oldExists {
		return
	}

	if !newExists {
		if err := os.Rename(oldPath, newPath); err == nil {
			logger.Logf("Moved legacy archive %s to %s.", oldPath, newPath)
			return
		} else {
			logger.Warn(fmt.Sprintf(
				"Could not move legacy archive %s to %s: %v. Will try to merge.",
				oldPath,
				newPath,
				err,
			))
		}
	}

	merged, err := mergeArchiveFiles(newPath, oldPath)
	if err != nil {
		logger.Warn(fmt.Sprintf(
			"Could not merge legacy archive %s into %s: %v",
			oldPath,
			newPath,
			err,
		))
		return
	}

	logger.Logf(
		"Merged %d unique archived ID(s) from legacy archive %s into %s.",
		merged,
		oldPath,
		newPath,
	)
}

func mergeArchiveFiles(dstPath, srcPath string) (int, error) {
	dstLines, err := fileio.ReadLinesToSet(dstPath)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	if dstLines == nil {
		dstLines = make(map[string]bool)
	}

	srcLines, err := fileio.ReadLinesToSet(srcPath)
	if err != nil {
		return 0, err
	}

	added := 0
	for line := range srcLines {
		if !dstLines[line] {
			dstLines[line] = true
			added++
		}
	}

	lines := make([]string, 0, len(dstLines))
	for line := range dstLines {
		lines = append(lines, line)
	}
	sort.Strings(lines)

	if err := fileio.WriteLines(dstPath, lines); err != nil {
		return 0, err
	}

	return added, nil
}
