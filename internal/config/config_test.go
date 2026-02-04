package config

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Days
		{"1d", 24 * time.Hour, false},
		{"14d", 14 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},

		// Weeks
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"4w", 28 * 24 * time.Hour, false},

		// Standard Go durations
		{"5m", 5 * time.Minute, false},
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"336h", 14 * 24 * time.Hour, false},
		{"1h30m", time.Hour + 30*time.Minute, false},

		// Edge cases
		{"0d", 0, false},
		{"0w", 0, false},
		{"", 0, false},
		{"  14d  ", 14 * 24 * time.Hour, false},

		// Errors
		{"invalid", 0, true},
		{"d", 0, true},
		{"w", 0, true},
		{"14x", 0, true},
		{"-1d", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
