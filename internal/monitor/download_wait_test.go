package monitor

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"os/exec"

	"streammon/internal/config"
	"streammon/internal/models"
	"streammon/internal/util/logging"
)

type testController struct {
	global *config.GlobalConfig
	stream config.StreamMonConfig
	prefix string
}

func (t *testController) GetGlobalConfig() *config.GlobalConfig       { return t.global }
func (t *testController) GetStreamMonConfig() *config.StreamMonConfig { return &t.stream }
func (t *testController) GetChannels() []config.Channel               { return nil }
func (t *testController) GetPollInterval() (time.Duration, error)     { return 0, nil }
func (t *testController) GetMaxRequestsPerSecond() float64            { return 0 }
func (t *testController) GetLogColor() string                         { return "" }
func (t *testController) GetLogPrefix() string                        { return t.prefix }
func (t *testController) CheckChannelStatus(ctx context.Context, ch config.Channel, httpClient *http.Client) (models.LiveInfo, error) {
	return models.LiveInfo{}, nil
}
func (t *testController) BuildDownloaderCmd(ch config.Channel, status models.LiveInfo) *exec.Cmd {
	return nil
}

// Test that a yt-dlp process which emitted a merger marker and left an output
// media file is considered successful even if the process exit code is non-zero,
// provided no postprocessing failure was detected.
func TestWaitForDownload_YTDLP_PostprocessDetection(t *testing.T) {
	initializeDownloadSlots(1)
	// Simulate that a slot was acquired so waitForDownload can release it.
	downloadSlots <- struct{}{}

	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "test.lock")
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.GetDefaultGlobalConfig()
	cfg.YoutubeArchiveDownloads = false
	cfg.SaveDownloadLogs = false
	cfg.SaveSystemLogs = false

	ctrl := &testController{global: cfg, stream: config.GetDefaultYTConfig().StreamMon, prefix: logPrefixYouTube}
	b := NewBaseMonitor(ctrl)

	// Prepare a command that exits non-zero. Use platform-appropriate shell.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", "exit", "1")
	} else {
		cmd = exec.Command("sh", "-c", "exit 1")
	}
	cmd.Dir = tmp
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Create a media file that matches the video ID so outputFileExists becomes true.
	videoID := "abc12345"
	mediaPath := filepath.Join(tmp, "video-"+videoID+".mp4")
	if err := os.WriteFile(mediaPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	proc := &downloadProcess{
		cmd:                   cmd,
		videoID:               videoID,
		downloaderName:        "yt-dlp",
		startedAt:             time.Now(),
		lockPath:              lockPath,
		logger:                logging.NewLogger(cfg, logPrefixYouTube, ""),
		isWaiting:             &atomic.Bool{},
		forcedTermination:     atomic.Bool{},
		mergerDetected:        &atomic.Bool{},
		downloadCompleted:     &atomic.Bool{},
		postprocessFailed:     &atomic.Bool{},
		fragmentFailure:       &atomic.Bool{},
		extractorFailed:       &atomic.Bool{},
		authFailure:           &atomic.Bool{},
		diskFailure:           &atomic.Bool{},
		processCrashed:        &atomic.Bool{},
		downloadWaitCount:     &atomic.Int32{},
		downloadWaitTriggered: &atomic.Bool{},
		status:                models.LiveInfo{},
	}
	proc.mergerDetected.Store(true)
	proc.postprocessFailed.Store(false)

	ch := config.Channel{ID: "chan1", Name: "TestChannel"}

	// Call waitForDownload which will wait for cmd to exit and then perform logic.
	b.waitForDownload(ch, proc)

	if !b.hasPendingYTSuccess(ch.ID, videoID) {
		t.Fatalf("expected pending YT success for %s", videoID)
	}
}

// Helper that mirrors the output parsing used in launchDownloader to set flags.
func applyOutputLine(proc *downloadProcess, line string) {
	line = logging.NormalizeSubprocessOutput(line)

	if strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats") || strings.Contains(line, "Successfully merged files into:") {
		proc.mergerDetected.Store(true)
	}
	if strings.Contains(line, "ERROR: Postprocessing:") || strings.Contains(line, "Postprocessing: Conversion failed") || strings.Contains(line, "Conversion failed") {
		proc.postprocessFailed.Store(true)
	}
	if strings.Contains(line, "Did not get any data blocks") || strings.Contains(line, "fragment not found") || strings.Contains(line, "Got error: HTTP Error") || (strings.Contains(line, "fragment") && strings.Contains(line, "Not Found")) {
		proc.fragmentFailure.Store(true)
	}
	if strings.Contains(line, "ERROR: Unable to download webpage") || strings.Contains(line, "ERROR: unable to extract") || strings.Contains(line, "ERROR: No video formats") || strings.Contains(line, "ERROR: unable to download video data") {
		proc.extractorFailed.Store(true)
	}
	if strings.Contains(line, "This video is private") || strings.Contains(line, "401 Unauthorized") || strings.Contains(line, "needs login") || strings.Contains(line, "requires authentication") {
		proc.authFailure.Store(true)
	}
	if strings.Contains(line, "Permission denied") || strings.Contains(line, "No space left on device") || strings.Contains(line, "file access error") {
		proc.diskFailure.Store(true)
	}
	if strings.Contains(line, "Killed") || strings.Contains(line, "segfault") || strings.Contains(line, "Traceback (most recent call last):") {
		proc.processCrashed.Store(true)
	}
}

func TestOutputDetectionFlags(t *testing.T) {
	proc := &downloadProcess{
		mergerDetected:    &atomic.Bool{},
		postprocessFailed: &atomic.Bool{},
		fragmentFailure:   &atomic.Bool{},
		extractorFailed:   &atomic.Bool{},
		authFailure:       &atomic.Bool{},
		diskFailure:       &atomic.Bool{},
		processCrashed:    &atomic.Bool{},
	}

	applyOutputLine(proc, "ERROR: Postprocessing: Conversion failed!")
	if !proc.postprocessFailed.Load() {
		t.Fatal("postprocessFailed not detected")
	}

	applyOutputLine(proc, "Did not get any data blocks while downloading fragments")
	if !proc.fragmentFailure.Load() {
		t.Fatal("fragmentFailure not detected")
	}

	applyOutputLine(proc, "ERROR: Unable to download webpage: HTTP Error 404")
	if !proc.extractorFailed.Load() {
		t.Fatal("extractorFailed not detected")
	}

	applyOutputLine(proc, "This video is private and requires authentication")
	if !proc.authFailure.Load() {
		t.Fatal("authFailure not detected")
	}

	applyOutputLine(proc, "No space left on device while writing file")
	if !proc.diskFailure.Load() {
		t.Fatal("diskFailure not detected")
	}

	applyOutputLine(proc, "Traceback (most recent call last):\n  File \"/usr/local/bin/yt-dlp\", line 10, in <module>")
	if !proc.processCrashed.Load() {
		t.Fatal("processCrashed not detected")
	}
}

func TestOutputDetectionFlagsIgnoreANSI(t *testing.T) {
	proc := &downloadProcess{
		mergerDetected:    &atomic.Bool{},
		postprocessFailed: &atomic.Bool{},
		fragmentFailure:   &atomic.Bool{},
		extractorFailed:   &atomic.Bool{},
		authFailure:       &atomic.Bool{},
		diskFailure:       &atomic.Bool{},
		processCrashed:    &atomic.Bool{},
	}

	applyOutputLine(proc, "\x1b[33m[Merger]\x1b[0m Merging formats")
	applyOutputLine(proc, "\x1b[31mERROR: Postprocessing: Conversion failed!\x1b[0m")
	applyOutputLine(proc, "\x1b[31mDid not get any data blocks\x1b[0m")
	applyOutputLine(proc, "\x1b[31mERROR: Unable to download webpage\x1b[0m")
	applyOutputLine(proc, "\x1b[31mThis video is private and requires authentication\x1b[0m")
	applyOutputLine(proc, "\x1b[31mNo space left on device\x1b[0m")
	applyOutputLine(proc, "\x1b[31mKilled\x1b[0m")

	if !proc.mergerDetected.Load() || !proc.postprocessFailed.Load() || !proc.fragmentFailure.Load() ||
		!proc.extractorFailed.Load() || !proc.authFailure.Load() || !proc.diskFailure.Load() || !proc.processCrashed.Load() {
		t.Fatal("expected ANSI-colored output to set every detection flag")
	}
}

// Fragment warnings (like "Did not get any data blocks") are often non-fatal
// in practice; ensure they don't block a successful outcome when merger and
// output file exist.
func TestFragmentDoesNotBlockSuccess(t *testing.T) {
	initializeDownloadSlots(1)
	downloadSlots <- struct{}{}

	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "test2.lock")
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.GetDefaultGlobalConfig()
	cfg.YoutubeArchiveDownloads = false
	cfg.SaveDownloadLogs = false
	cfg.SaveSystemLogs = false

	ctrl := &testController{global: cfg, stream: config.GetDefaultYTConfig().StreamMon, prefix: logPrefixYouTube}
	b := NewBaseMonitor(ctrl)

	// Start a command that exits non-zero but we'll mark merger and create file.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", "exit", "1")
	} else {
		cmd = exec.Command("sh", "-c", "exit 1")
	}
	cmd.Dir = tmp
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	videoID := "frag12345"
	mediaPath := filepath.Join(tmp, "video-"+videoID+".mp4")
	if err := os.WriteFile(mediaPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	proc := &downloadProcess{
		cmd:                   cmd,
		videoID:               videoID,
		downloaderName:        "yt-dlp",
		startedAt:             time.Now(),
		lockPath:              lockPath,
		logger:                logging.NewLogger(cfg, logPrefixYouTube, ""),
		mergerDetected:        &atomic.Bool{},
		postprocessFailed:     &atomic.Bool{},
		fragmentFailure:       &atomic.Bool{},
		extractorFailed:       &atomic.Bool{},
		authFailure:           &atomic.Bool{},
		diskFailure:           &atomic.Bool{},
		processCrashed:        &atomic.Bool{},
		downloadCompleted:     &atomic.Bool{},
		downloadWaitCount:     &atomic.Int32{},
		downloadWaitTriggered: &atomic.Bool{},
		status:                models.LiveInfo{},
	}
	proc.mergerDetected.Store(true)
	proc.fragmentFailure.Store(true)

	ch := config.Channel{ID: "chan2", Name: "FragChannel"}
	b.waitForDownload(ch, proc)

	if !b.hasPendingYTSuccess(ch.ID, videoID) {
		t.Fatalf("expected pending YT success for %s despite fragment failures", videoID)
	}
}
