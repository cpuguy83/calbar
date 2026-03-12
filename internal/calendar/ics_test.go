package calendar

import (
	"strings"
	"testing"
	"time"

	ics "github.com/emersion/go-ical"
)

func withLocalTimezone(t *testing.T, loc *time.Location) {
	t.Helper()

	prev := time.Local
	time.Local = loc
	t.Cleanup(func() {
		time.Local = prev
	})
}

func TestParseEvent_EffectivelyAllDay(t *testing.T) {
	// Simulate an iCloud-style multi-day event encoded with full datetimes at midnight
	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-multiday-allday
SUMMARY:Mid-winter break (no school)
DTSTART:20260216T000000
DTEND:20260221T000000
END:VEVENT
END:VCALENDAR`

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{
		name: "test",
		end:  time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local),
	}

	for _, child := range cal.Children {
		if child.Name != ics.CompEvent {
			continue
		}
		events, err := s.parseEvent(child)
		if err != nil {
			t.Fatalf("parseEvent error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		ev := events[0]
		if !ev.AllDay {
			t.Errorf("expected AllDay=true for midnight-to-midnight multi-day event, got false")
		}
		if ev.Summary != "Mid-winter break (no school)" {
			t.Errorf("unexpected summary: %s", ev.Summary)
		}
	}
}

func TestParseEvent_DateOnlyAllDay(t *testing.T) {
	// Standard date-only all-day event
	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-dateonly-allday
SUMMARY:Holiday
DTSTART;VALUE=DATE:20260217
DTEND;VALUE=DATE:20260218
END:VEVENT
END:VCALENDAR`

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{
		name: "test",
		end:  time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local),
	}

	for _, child := range cal.Children {
		if child.Name != ics.CompEvent {
			continue
		}
		events, err := s.parseEvent(child)
		if err != nil {
			t.Fatalf("parseEvent error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if !events[0].AllDay {
			t.Errorf("expected AllDay=true for date-only event, got false")
		}
	}
}

func TestParseEvent_TimedEventNotAllDay(t *testing.T) {
	// Normal timed event should NOT be marked all-day
	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-timed
SUMMARY:Meeting
DTSTART:20260217T100000
DTEND:20260217T110000
END:VEVENT
END:VCALENDAR`

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{
		name: "test",
		end:  time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local),
	}

	for _, child := range cal.Children {
		if child.Name != ics.CompEvent {
			continue
		}
		events, err := s.parseEvent(child)
		if err != nil {
			t.Fatalf("parseEvent error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].AllDay {
			t.Errorf("expected AllDay=false for timed event, got true")
		}
	}
}

func TestParseEvent_WindowsTZIDConvertedToLocalTime(t *testing.T) {
	gmt := time.FixedZone("GMT", 0)
	withLocalTimezone(t, gmt)

	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-windows-tzid
SUMMARY:Remote meeting
DTSTART;TZID=GTB Standard Time:20260219T124000
DTEND;TZID=GTB Standard Time:20260219T131000
END:VEVENT
END:VCALENDAR`

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{
		name: "test",
		end:  time.Date(2026, 3, 1, 0, 0, 0, 0, gmt),
	}

	for _, child := range cal.Children {
		if child.Name != ics.CompEvent {
			continue
		}

		events, err := s.parseEvent(child)
		if err != nil {
			t.Fatalf("parseEvent error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		wantStart := time.Date(2026, 2, 19, 10, 40, 0, 0, gmt)
		wantEnd := time.Date(2026, 2, 19, 11, 10, 0, 0, gmt)
		if got := events[0].Start.In(gmt); !got.Equal(wantStart) {
			t.Fatalf("unexpected start: got %s want %s", got, wantStart)
		}
		if got := events[0].End.In(gmt); !got.Equal(wantEnd) {
			t.Fatalf("unexpected end: got %s want %s", got, wantEnd)
		}
	}
}
