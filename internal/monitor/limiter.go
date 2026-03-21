package monitor

import (
	"os/exec"
	"sync"
	"sync/atomic"

	"streammon/internal/util"
)

// downloadProcess holds information about a running download process.
type downloadProcess struct {
	cmd       *exec.Cmd
	videoID   string
	lockPath  string
	logger    *util.DownloadLogger
	isWaiting *atomic.Bool // Signals that the process is in a waiting/retry state
}

// --- Global Download Limiter ---

var (
	downloadSlots     chan struct{}
	downloadSlotsOnce sync.Once
)

// initializeDownloadSlots creates the global semaphore for limiting concurrent downloads.
// It's safe to call multiple times; it will only initialize the semaphore once.
func initializeDownloadSlots(max int) {
	downloadSlotsOnce.Do(func() {
		// Ensure at least one download is possible, even if config is 0 or less.
		if max <= 0 {
			max = 1
		}
		downloadSlots = make(chan struct{}, max)
	})
}
