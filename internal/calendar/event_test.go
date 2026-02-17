package calendar

import (
	"testing"
	"time"
)

func TestIsEffectivelyAllDay(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  bool
	}{
		{
			name:  "single day midnight to midnight",
			start: time.Date(2026, 2, 17, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 18, 0, 0, 0, 0, loc),
			want:  true,
		},
		{
			name:  "multi-day midnight to midnight (5 days)",
			start: time.Date(2026, 2, 16, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 21, 0, 0, 0, 0, loc),
			want:  true,
		},
		{
			name:  "start not midnight",
			start: time.Date(2026, 2, 17, 9, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 18, 0, 0, 0, 0, loc),
			want:  false,
		},
		{
			name:  "end not midnight",
			start: time.Date(2026, 2, 17, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 18, 17, 0, 0, 0, loc),
			want:  false,
		},
		{
			name:  "same time (zero duration)",
			start: time.Date(2026, 2, 17, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 17, 0, 0, 0, 0, loc),
			want:  false,
		},
		{
			name:  "end before start",
			start: time.Date(2026, 2, 18, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 2, 17, 0, 0, 0, 0, loc),
			want:  false,
		},
		{
			name:  "start has seconds",
			start: time.Date(2026, 2, 17, 0, 0, 1, 0, loc),
			end:   time.Date(2026, 2, 18, 0, 0, 0, 0, loc),
			want:  false,
		},
		{
			name:  "normal timed event",
			start: time.Date(2026, 2, 17, 10, 30, 0, 0, loc),
			end:   time.Date(2026, 2, 17, 11, 30, 0, 0, loc),
			want:  false,
		},
		{
			name:  "non-local timezone midnight not local midnight",
			start: time.Date(2026, 2, 17, 5, 0, 0, 0, loc), // 5am local
			end:   time.Date(2026, 2, 18, 5, 0, 0, 0, loc), // 5am local
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEffectivelyAllDay(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("isEffectivelyAllDay(%v, %v) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}
