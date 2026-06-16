package monitor

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"streammon/internal/models"
	"streammon/internal/util/logging"
)

// downloadProcess holds information about a running download process.
type downloadProcess struct {
	cmd                   *exec.Cmd
	videoID               string
	downloaderName        string
	previousDownloader    string
	startedAt             time.Time
	lockPath              string
	logger                *logging.Logger
	isWaiting             *atomic.Bool // Signals that the process is in a waiting/retry state
	forcedTermination     atomic.Bool  // Signals that the monitor intentionally stopped the process
	mergerDetected        *atomic.Bool // Tracks if [Merger] or successful completion marker was detected in output
	downloadCompleted     *atomic.Bool // Tracks downloader-specific completion markers in output
	postprocessFailed     *atomic.Bool // Tracks if post-processing (ffmpeg/merger) reported a failure
	fragmentFailure       *atomic.Bool // Tracks repeated fragment/network failures escalated by output
	extractorFailed       *atomic.Bool // Tracks extractor or extraction failures
	authFailure           *atomic.Bool // Tracks authentication/permission failures
	diskFailure           *atomic.Bool // Tracks disk/write/permission failures
	processCrashed        *atomic.Bool // Tracks if subprocess crashed or was killed unexpectedly
	downloadWaitCount     *atomic.Int32
	downloadWaitTriggered *atomic.Bool
	status                models.LiveInfo
	outputCallback        func(string)
	fallbackAttempted     bool
	retryMode             string
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
