package calendar

import (
	"testing"
	"time"
)

func TestConvertEvent_UsesGraphReminder(t *testing.T) {
	s := &MS365Source{name: "ms365"}

	event, err := s.convertEvent(graphEvent{
		ID:              "id-1",
		Subject:         "Reminder event",
		Start:           graphDateTime{DateTime: "2026-02-19T12:00:00.0000000", TimeZone: "UTC"},
		End:             graphDateTime{DateTime: "2026-02-19T13:00:00.0000000", TimeZone: "UTC"},
		IsReminderOn:    true,
		ReminderMinutes: 15,
	})
	if err != nil {
		t.Fatalf("convertEvent error: %v", err)
	}

	if len(event.NotifyAt) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(event.NotifyAt))
	}

	want := time.Date(2026, 2, 19, 11, 45, 0, 0, time.UTC)
	if got := event.NotifyAt[0]; !got.Equal(want) {
		t.Fatalf("unexpected reminder time: got %s want %s", got, want)
	}
}
