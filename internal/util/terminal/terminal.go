package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"streammon/internal/util/ansi"
)

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

	fmt.Println(ansi.ColorBlue + strings.Repeat("=", 60) + ansi.ColorReset)
	fmt.Printf(" %sstreammon%s - Automated Stream Archiver\n", ansi.ColorGreen, ansi.ColorReset)
	fmt.Println(ansi.ColorBlue + strings.Repeat("=", 60) + ansi.ColorReset)
}
