package monitor

import (
	"reflect"
	"testing"
)

func TestTwitchDownloaderArgs(t *testing.T) {
	tests := []struct {
		name        string
		globalArgs  []string
		channelArgs []string
		want        []string
	}{
		{
			name:        "strips live-from-start and appends channel args",
			globalArgs:  []string{"--live-from-start", "--retry-streams", "60"},
			channelArgs: []string{"--strip-live-from-start", "--output", "channel.mp4"},
			want:        []string{"--retry-streams", "60", "--output", "channel.mp4"},
		},
		{
			name:        "keeps global live-from-start without control token",
			globalArgs:  []string{"--live-from-start", "--retry-streams", "60"},
			channelArgs: []string{"--output", "channel.mp4"},
			want:        []string{"--live-from-start", "--retry-streams", "60", "--output", "channel.mp4"},
		},
		{
			name:        "control token is harmless when global flag is absent",
			globalArgs:  []string{"--retry-streams", "60"},
			channelArgs: []string{"--strip-live-from-start"},
			want:        []string{"--retry-streams", "60"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := twitchDownloaderArgs(tt.globalArgs, tt.channelArgs); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("twitchDownloaderArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
