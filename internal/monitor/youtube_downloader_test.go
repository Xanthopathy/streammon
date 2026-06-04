package monitor

import (
	"strings"
	"testing"
)

func TestRemoveLiveWaitArgs(t *testing.T) {
	args := []string{
		"--wait-for-video", "60",
		"--live-from-start",
		"--wait-for-video=30",
		"--retries", "10",
		"--output", "video.%(ext)s",
	}

	got := removeLiveWaitArgs(args)
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--live-from-start") || strings.Contains(joined, "--wait-for-video") || strings.Contains(joined, " 60 ") {
		t.Fatalf("removeLiveWaitArgs() left live wait args: %v", got)
	}
	if !strings.Contains(joined, "--retries 10") {
		t.Fatalf("removeLiveWaitArgs() removed unrelated args: %v", got)
	}
}

func TestBuildTimestampedOutputArgsInsertsBeforeExtension(t *testing.T) {
	got := buildTimestampedOutputArgs([]string{"--output", "video.%(ext)s"}, "retry")

	if len(got) != 2 || !strings.HasPrefix(got[1], "video [retry-") || !strings.HasSuffix(got[1], "].%(ext)s") {
		t.Fatalf("buildTimestampedOutputArgs() = %v", got)
	}
}
