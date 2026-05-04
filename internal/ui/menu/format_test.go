package menu

import (
	"strings"
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

func TestFormatEventDetails_ShowsEventReminderDetails(t *testing.T) {
	start := time.Date(2026, 2, 17, 10, 0, 0, 0, time.Local)
	lines, _ := formatEventDetails(&calendar.Event{
		Summary:  "Standup",
		Start:    start,
		End:      start.Add(30 * time.Minute),
		NotifyAt: []time.Time{start.Add(-15 * time.Minute)},
	}, nil)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Using event reminders") {
		t.Fatalf("expected event reminder label, got %q", joined)
	}
	if !strings.Contains(joined, "15 minutes before start") {
		t.Fatalf("expected reminder offset, got %q", joined)
	}
}

func TestFormatEventDetails_ShowsOverrideReminderDetails(t *testing.T) {
	start := time.Date(2026, 2, 17, 10, 0, 0, 0, time.Local)
	lines, _ := formatEventDetails(&calendar.Event{
		Summary:  "Standup",
		Start:    start,
		End:      start.Add(30 * time.Minute),
		NotifyAt: []time.Time{start.Add(-15 * time.Minute)},
	}, []time.Duration{0})

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Using config override") {
		t.Fatalf("expected override label, got %q", joined)
	}
	if !strings.Contains(joined, "at start") {
		t.Fatalf("expected at-start reminder, got %q", joined)
	}
}

func TestFormatEventDetails_ShowsMeetingDetails(t *testing.T) {
	start := time.Date(2026, 5, 5, 12, 0, 0, 0, time.Local)
	lines, urlMap := formatEventDetails(&calendar.Event{
		Summary:  "Teams meeting",
		Start:    start,
		End:      start.Add(time.Hour),
		Location: "Conference Room",
		Meeting: calendar.MeetingDetails{
			URL:               "https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6",
			Service:           "Microsoft Teams Meeting",
			ID:                "227 921 734 315 68",
			Passcode:          "Wi6P69wc",
			DialIn:            "+1 323-849-4874,,864359718# United States, Los Angeles",
			PhoneConferenceID: "864 359 718#",
		},
	}, nil)

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"Microsoft Teams Meeting",
		"Meeting ID: 227 921 734 315 68",
		"Passcode: Wi6P69wc",
		"Dial-in: +1 323-849-4874,,864359718# United States, Los Angeles",
		"Phone conference ID: 864 359 718#",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in details, got %q", want, joined)
		}
	}
	if got := urlMap["🔗 Join Teams Meeting"]; got != "https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6" {
		t.Fatalf("unexpected join URL: %q", got)
	}
}
