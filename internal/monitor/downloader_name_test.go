package monitor

import "testing"

func TestDownloaderNameFromCommandNormalizesWindowsExecutables(t *testing.T) {
	tests := []struct {
		name string
		path string
		args []string
		want string
	}{
		{
			name: "yt-dlp exe",
			path: `D:\tools\yt-dlp.exe`,
			args: []string{"yt-dlp", "https://www.youtube.com/watch?v=abc"},
			want: "yt-dlp",
		},
		{
			name: "livestream_dl exe",
			path: `D:\tools\livestream_dl.exe`,
			args: []string{"livestream_dl", "abc"},
			want: "livestream_dl",
		},
		{
			name: "npx twitch-dlp cmd",
			path: `C:\Program Files\nodejs\npx.cmd`,
			args: []string{"npx", "-y", "twitch-dlp", "https://www.twitch.tv/example"},
			want: "twitch-dlp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := downloaderNameFromCommand(tt.path, tt.args); got != tt.want {
				t.Fatalf("downloaderNameFromCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
