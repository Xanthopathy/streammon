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
			name: "ANSI-colored yt-dlp download",
			line: "\x1b[32m[download]\x1b[0m 245.96MiB at 2.81MiB/s",
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
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "wait", line: "[wait] Waiting for video", want: true},
		{name: "retry streams", line: "[retry-streams] Waiting for stream", want: true},
		{name: "ANSI-colored wait", line: "\x1b[33m[wait]\x1b[0m Waiting for video", want: true},
		{name: "no-longer-live warning", line: "WARNING: [youtube] Vm7xAYtmZcE: Video is no longer live. Retrying (1/3)...", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSubprocessWaitLine(tt.line); got != tt.want {
				t.Fatalf("IsSubprocessWaitLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsYouTubeNoLongerLiveWarning(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "plain warning",
			line: "WARNING: [youtube] Vm7xAYtmZcE: Video is no longer live. Retrying (1/3)...",
			want: true,
		},
		{
			name: "ANSI-colored warning",
			line: "\x1b[33mWARNING:\x1b[0m [youtube] Vm7xAYtmZcE: Video is no longer live. Retrying (1/3)...",
			want: true,
		},
		{
			name: "unrelated YouTube warning",
			line: "WARNING: [youtube] Unable to download webpage",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsYouTubeNoLongerLiveWarning(tt.line); got != tt.want {
				t.Fatalf("IsYouTubeNoLongerLiveWarning() = %v, want %v", got, tt.want)
			}
		})
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
	if got, want := colorizeLivestreamDLOutput(warning), "["+ansi.ColorYellow+"WARNING"+ansi.ColorReset+"] [5a5f6URAlIs] segment failed"; got != want {
		t.Fatalf("expected only warning text to be yellow, got %q", got)
	}

	info := "[INFO] [5a5f6URAlIs] merge complete"
	if got, want := colorizeLivestreamDLOutput(info), "["+ansi.ColorBlue+"INFO"+ansi.ColorReset+"] [5a5f6URAlIs] merge complete"; got != want {
		t.Fatalf("expected only info text to be blue, got %q", got)
	}
}
