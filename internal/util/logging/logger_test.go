package logging

import "testing"

func TestFormatEventTag(t *testing.T) {
	tests := map[string]string{
		"STARTUP":    "Startup",
		"download":   "Download",
		" Retry ":    "Retry",
		"DIAGNOSTIC": "Diagnostic",
		"":           "",
	}

	for input, want := range tests {
		if got := formatEventTag(input); got != want {
			t.Fatalf("formatEventTag(%q) = %q, want %q", input, got, want)
		}
	}
}
