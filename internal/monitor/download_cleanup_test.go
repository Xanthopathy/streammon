package monitor

import "testing"

func TestIsYTDLPResidueFile(t *testing.T) {
	const videoID = "Vm7xAYtmZcE"

	tests := []struct {
		name           string
		fileName       string
		downloaderName string
		want           bool
	}{
		{
			name:           "part file",
			fileName:       "[20260609] [Vm7xAYtmZcE] [title].f299.mp4.part",
			downloaderName: "yt-dlp",
			want:           true,
		},
		{
			name:           "fragment part file",
			fileName:       "[20260609] [Vm7xAYtmZcE] [title].f299.mp4.part-Frag2511.part",
			downloaderName: "yt-dlp",
			want:           true,
		},
		{
			name:           "resume metadata file",
			fileName:       "[20260609] [Vm7xAYtmZcE] [title].f299.mp4.ytdl",
			downloaderName: "yt-dlp",
			want:           true,
		},
		{
			name:           "final media file",
			fileName:       "[20260609] [Vm7xAYtmZcE] [title].mp4",
			downloaderName: "yt-dlp",
			want:           false,
		},
		{
			name:           "different video",
			fileName:       "[20260609] [different] [title].f299.mp4.part",
			downloaderName: "yt-dlp",
			want:           false,
		},
		{
			name:           "not yt-dlp",
			fileName:       "[20260609] [Vm7xAYtmZcE] [title].f299.mp4.part",
			downloaderName: "livestream_dl",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isYTDLPResidueFile(tt.fileName, videoID, tt.downloaderName)
			if got != tt.want {
				t.Fatalf("isYTDLPResidueFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
