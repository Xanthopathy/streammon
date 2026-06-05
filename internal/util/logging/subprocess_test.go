package logging

import (
	"strings"
	"testing"

	"streammon/internal/util/ansi"
)

func TestIsSubprocessProgressLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "yt-dlp download",
			line: "[download] 245.96MiB at 2.81MiB/s",
			want: true,
		},
		{
			name: "livestream_dl text stats",
			line: "8F22yBCpXRc: Video: 19474/19472 (Recording) Audio: 19474/19472 (Recording) ~10.55 GB downloaded",
			want: true,
		},
		{
			name: "livestream_dl json stats",
			line: `{"id":"8F22yBCpXRc","video":{"downloaded_segments":10},"audio":{"downloaded_segments":10}}`,
			want: true,
		},
		{
			name: "livestream_dl info",
			line: "[INFO] [8F22yBCpXRc] 2026-06-04 03:40:01 - Successfully merged files into: output.mp4",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSubprocessProgressLine(tt.line); got != tt.want {
				t.Fatalf("IsSubprocessProgressLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSubprocessWaitLine(t *testing.T) {
	if !IsSubprocessWaitLine("[wait] Waiting for video") {
		t.Fatal("expected [wait] line to be classified as wait")
	}
	if !IsSubprocessWaitLine("[retry-streams] Waiting for stream") {
		t.Fatal("expected [retry-streams] line to be classified as wait")
	}
}

func TestColorizeSubprocessOutputOnlyTouchesLivestreamDL(t *testing.T) {
	line := "5a5f6URAlIs: Video: 5195/5194 (Recording) Audio: 5193/5191 (Recording) ~657.55 MB downloaded"

	got := colorizeSubprocessOutput("livestream_dl", line)
	wantAmount := ansi.ColorBlue + "~657.55 MB downloaded" + ansi.ColorReset
	if !strings.Contains(got, wantAmount) {
		t.Fatalf("expected downloaded amount to be blue, got %q", got)
	}

	got = colorizeSubprocessOutput("yt-dlp", line)
	if got != line {
		t.Fatalf("expected non-livestream_dl output to remain unchanged, got %q", got)
	}
}

func TestColorizeLivestreamDLSeverityTags(t *testing.T) {
	warning := "[WARNING] [5a5f6URAlIs] segment failed"
	if got := colorizeLivestreamDLOutput(warning); !strings.Contains(got, ansi.ColorYellow+"[WARNING]"+ansi.ColorReset) {
		t.Fatalf("expected warning tag to be yellow, got %q", got)
	}

	info := "[INFO] [5a5f6URAlIs] merge complete"
	if got := colorizeLivestreamDLOutput(info); !strings.Contains(got, ansi.ColorBlue+"[INFO]"+ansi.ColorReset) {
		t.Fatalf("expected info tag to be blue, got %q", got)
	}
}
