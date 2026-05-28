package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"streammon/internal/util/text"
)

func GetLockfilePath(workDir, channelName, id string) string {
	sanitizedName := text.SanitizeFolderName(channelName)
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

// ClearLockfiles removes all files starting with ".lock-" in the specified directory.
// Returns the number of files deleted.
func ClearLockfiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".lock-") {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err == nil {
				count++
			}
		}
	}
	return count, nil
}
