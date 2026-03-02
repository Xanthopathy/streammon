package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// --- Colors ---
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[91m"
	ColorGreen  = "\033[92m"
	ColorYellow = "\033[93m"
	ColorBlue   = "\033[94m"
	ColorCyan   = "\033[96m"
)

// --- UI Helpers ---

func PrintBanner() {
	// Clear console
	cmd := exec.Command("cmd", "/c", "cls")
	if runtime.GOOS != "windows" {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Println(ColorBlue + strings.Repeat("=", 60) + ColorReset)
	fmt.Printf(" %sStreamMon (Go)%s - Automated Stream Archiver\n", ColorGreen, ColorReset)
	fmt.Println(ColorBlue + strings.Repeat("=", 60) + ColorReset)
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
