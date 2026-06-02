package utcoffset

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		offset  string
		want    int
		wantErr bool
	}{
		{name: "empty", offset: "", want: 0},
		{name: "utc", offset: "UTC", want: 0},
		{name: "positive hours", offset: "+7", want: 7 * 3600},
		{name: "negative hours", offset: "-5", want: -5 * 3600},
		{name: "utc prefix", offset: "UTC+7", want: 7 * 3600},
		{name: "minutes", offset: "+5:30", want: 5*3600 + 30*60},
		{name: "trimmed", offset: " UTC-3:15 ", want: -(3*3600 + 15*60)},
		{name: "hour out of range", offset: "+24", wantErr: true},
		{name: "minute out of range", offset: "+1:60", wantErr: true},
		{name: "invalid", offset: "wat", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.offset)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) returned nil error", tt.offset)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.offset, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %d, want %d", tt.offset, got, tt.want)
			}
		})
	}
}
