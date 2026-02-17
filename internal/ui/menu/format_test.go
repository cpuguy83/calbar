package menu

import (
	"testing"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
)

func TestFormatAllDayRange(t *testing.T) {
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.Local)

	tests := []struct {
		name      string
		event     *calendar.Event
		wantEmpty bool
	}{
		{
			name: "single day all-day event returns empty",
			event: &calendar.Event{
				Start: time.Date(2026, 2, 17, 0, 0, 0, 0, time.Local),
				End:   time.Date(2026, 2, 18, 0, 0, 0, 0, time.Local),
			},
			wantEmpty: true,
		},
		{
			name: "multi-day all-day event returns range",
			event: &calendar.Event{
				Start: time.Date(2026, 2, 16, 0, 0, 0, 0, time.Local),
				End:   time.Date(2026, 2, 21, 0, 0, 0, 0, time.Local),
			},
			wantEmpty: false,
		},
		{
			name: "two-day all-day event returns range",
			event: &calendar.Event{
				Start: time.Date(2026, 2, 17, 0, 0, 0, 0, time.Local),
				End:   time.Date(2026, 2, 19, 0, 0, 0, 0, time.Local),
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAllDayRange(tt.event, now)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("expected non-empty date range string, got empty")
			}
		})
	}
}

func TestFormatAllDayRange_UsesRelativeDays(t *testing.T) {
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.Local)

	// Event spanning today through tomorrow
	e := &calendar.Event{
		Start: time.Date(2026, 2, 17, 0, 0, 0, 0, time.Local),
		End:   time.Date(2026, 2, 19, 0, 0, 0, 0, time.Local),
	}

	got := formatAllDayRange(e, now)
	if got != "Today – Tomorrow" {
		t.Errorf("expected 'Today – Tomorrow', got %q", got)
	}
}
