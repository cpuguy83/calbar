package calendar

import (
	"fmt"
	"path/filepath"
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

func TestWriteReadICS_PreservesMeetingDetails(t *testing.T) {
	start := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	events := []Event{{
		UID:         "meeting-1",
		Summary:     "Meeting",
		Description: "Useful description",
		Start:       start,
		End:         start.Add(time.Hour),
		Source:      "ms365",
		Meeting: MeetingDetails{
			URL:               "https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6",
			Service:           "Microsoft Teams Meeting",
			ID:                "227 921 734 315 68",
			Passcode:          "Wi6P69wc",
			DialIn:            "+1 323-849-4874,,864359718# United States, Los Angeles",
			PhoneConferenceID: "864 359 718#",
		},
	}}

	path := filepath.Join(t.TempDir(), "events.ics")
	if err := WriteICS(path, events); err != nil {
		t.Fatalf("WriteICS error: %v", err)
	}

	parsed, err := ReadICS(path)
	if err != nil {
		t.Fatalf("ParseICS error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 event, got %d", len(parsed))
	}
	if parsed[0].Meeting != events[0].Meeting {
		t.Fatalf("unexpected meeting details: got %#v want %#v", parsed[0].Meeting, events[0].Meeting)
	}
}

func TestParseICS_DescriptionUnescapesLiteralNewlines(t *testing.T) {
	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-cache-description-newlines
SUMMARY:Meeting
DESCRIPTION:Line one\nLine two\NLine three
DTSTART:20260217T100000
DTEND:20260217T110000
END:VEVENT
END:VCALENDAR`

	events, err := ParseICS(strings.NewReader(icsData))
	if err != nil {
		t.Fatalf("ParseICS error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	want := "Line one\nLine two\nLine three"
	if got := events[0].Description; got != want {
		t.Fatalf("unexpected description: got %q want %q", got, want)
	}
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

func TestParseEvent_DescriptionUnescapesLiteralNewlines(t *testing.T) {
	icsData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-description-newlines
SUMMARY:Meeting
DESCRIPTION:Line one\nLine two\NLine three\, with comma\; and semicolon
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
		want := "Line one\nLine two\nLine three, with comma; and semicolon"
		if got := events[0].Description; got != want {
			t.Fatalf("unexpected description: got %q want %q", got, want)
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

func TestParseEvent_DisplayAlarmTrigger(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	icsData := fmt.Sprintf(`BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-display-alarm
SUMMARY:Meeting
DTSTART:%s
DTEND:%s
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER:-PT15M
END:VALARM
END:VEVENT
END:VCALENDAR`, start.Format("20060102T150405"), end.Format("20060102T150405"))

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{name: "test", end: end.Add(24 * time.Hour)}

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
		if len(events[0].NotifyAt) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(events[0].NotifyAt))
		}

		want := start.Add(-15 * time.Minute)
		if got := events[0].NotifyAt[0]; !got.Equal(want) {
			t.Fatalf("unexpected reminder time: got %s want %s", got, want)
		}
	}
}

func TestParseEvent_IgnoresNonDisplayAlarm(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	icsData := fmt.Sprintf(`BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-email-alarm
SUMMARY:Meeting
DTSTART:%s
DTEND:%s
BEGIN:VALARM
ACTION:EMAIL
TRIGGER:-PT15M
END:VALARM
END:VEVENT
END:VCALENDAR`, start.Format("20060102T150405"), end.Format("20060102T150405"))

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{name: "test", end: end.Add(24 * time.Hour)}

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
		if len(events[0].NotifyAt) != 0 {
			t.Fatalf("expected no reminders, got %d", len(events[0].NotifyAt))
		}
	}
}

func TestParseEvent_RecurringDisplayAlarmUsesOccurrenceStart(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	until := start.Add(48 * time.Hour)

	icsData := fmt.Sprintf(`BEGIN:VCALENDAR
BEGIN:VEVENT
UID:test-recurring-alarm
SUMMARY:Recurring meeting
DTSTART:%s
DTEND:%s
RRULE:FREQ=DAILY;COUNT=2
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER:-PT10M
END:VALARM
END:VEVENT
END:VCALENDAR`, start.Format("20060102T150405"), end.Format("20060102T150405"))

	dec := ics.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode ICS: %v", err)
	}

	s := &ICSSource{name: "test", end: until}

	for _, child := range cal.Children {
		if child.Name != ics.CompEvent {
			continue
		}

		events, err := s.parseEvent(child)
		if err != nil {
			t.Fatalf("parseEvent error: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}

		for i, event := range events {
			if len(event.NotifyAt) != 1 {
				t.Fatalf("event %d expected 1 reminder, got %d", i, len(event.NotifyAt))
			}
			want := event.Start.Add(-10 * time.Minute)
			if got := event.NotifyAt[0]; !got.Equal(want) {
				t.Fatalf("event %d unexpected reminder time: got %s want %s", i, got, want)
			}
		}
	}
}
