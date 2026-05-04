package calendar

import (
	"strings"
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

func TestConvertEvent_ParsesTeamsFooterIntoMeetingDetails(t *testing.T) {
	s := &MS365Source{name: "ms365"}

	event, err := s.convertEvent(graphEvent{
		ID:      "id-1",
		Subject: "Teams event",
		Start:   graphDateTime{DateTime: "2026-05-05T12:00:00.0000000", TimeZone: "UTC"},
		End:     graphDateTime{DateTime: "2026-05-05T13:00:00.0000000", TimeZone: "UTC"},
		Body: &graphBody{Content: `What this is
Welcome to the lab series.

Resources
* https://aka.ms/IcMAgentStudioDemo

________________________________________________________________________________
Microsoft Teams meeting
Join: https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6
Meeting ID: 227 921 734 315 68
Passcode: Wi6P69wc
________________________________
Need help? | System reference
Dial in by phone
+1 323-849-4874,,864359718# United States, Los Angeles
Find a local number
Phone conference ID: 864 359 718#
For organizers: Meeting options | Reset dial-in PIN
________________________________________________________________________________`},
	})
	if err != nil {
		t.Fatalf("convertEvent error: %v", err)
	}

	if event.Location != "Microsoft Teams Meeting" {
		t.Fatalf("unexpected location: %q", event.Location)
	}
	if event.Meeting.URL != "https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6" {
		t.Fatalf("unexpected meeting URL: %q", event.Meeting.URL)
	}
	if event.Meeting.Service != "Microsoft Teams Meeting" {
		t.Fatalf("unexpected meeting service: %q", event.Meeting.Service)
	}
	if event.Meeting.ID != "227 921 734 315 68" {
		t.Fatalf("unexpected meeting ID: %q", event.Meeting.ID)
	}
	if event.Meeting.Passcode != "Wi6P69wc" {
		t.Fatalf("unexpected passcode: %q", event.Meeting.Passcode)
	}
	if event.Meeting.DialIn != "+1 323-849-4874,,864359718# United States, Los Angeles" {
		t.Fatalf("unexpected dial-in: %q", event.Meeting.DialIn)
	}
	if event.Meeting.PhoneConferenceID != "864 359 718#" {
		t.Fatalf("unexpected phone conference ID: %q", event.Meeting.PhoneConferenceID)
	}
	if !strings.Contains(event.Description, "Welcome to the lab series.") {
		t.Fatalf("expected description content, got %q", event.Description)
	}
	if strings.Contains(event.Description, "Microsoft Teams meeting") || strings.Contains(event.Description, "Meeting ID:") {
		t.Fatalf("expected Teams footer to be removed, got %q", event.Description)
	}
	if !strings.Contains(event.Description, "\n") {
		t.Fatalf("expected description line breaks to be preserved, got %q", event.Description)
	}
}

func TestConvertEvent_OnlineMeetingURLOverridesBodyURL(t *testing.T) {
	s := &MS365Source{name: "ms365"}

	event, err := s.convertEvent(graphEvent{
		ID:      "id-1",
		Subject: "Teams event",
		Start:   graphDateTime{DateTime: "2026-05-05T12:00:00.0000000", TimeZone: "UTC"},
		End:     graphDateTime{DateTime: "2026-05-05T13:00:00.0000000", TimeZone: "UTC"},
		Body: &graphBody{Content: `Body text

Microsoft Teams meeting
Join: https://teams.microsoft.com/meet/body-url?p=body`},
		OnlineMeeting: &graphOnlineMeeting{JoinURL: "https://teams.microsoft.com/l/meetup-join/structured-url"},
	})
	if err != nil {
		t.Fatalf("convertEvent error: %v", err)
	}

	if event.Meeting.URL != "https://teams.microsoft.com/l/meetup-join/structured-url" {
		t.Fatalf("unexpected meeting URL: %q", event.Meeting.URL)
	}
	if strings.Contains(event.Description, "https://teams.microsoft.com") {
		t.Fatalf("expected Teams footer URL removed from description, got %q", event.Description)
	}
}

func TestConvertEvent_RemovesTeamsNeedHelpFooter(t *testing.T) {
	s := &MS365Source{name: "ms365"}

	event, err := s.convertEvent(graphEvent{
		ID:      "id-1",
		Subject: "Teams event",
		Start:   graphDateTime{DateTime: "2026-05-05T12:00:00.0000000", TimeZone: "UTC"},
		End:     graphDateTime{DateTime: "2026-05-05T13:00:00.0000000", TimeZone: "UTC"},
		Body: &graphBody{Content: `Adding meeting room for folks in Redmond
Adding a separate triage meeting to assign RCAs and discussion of ICMs in our queue. Might have missed some people - please add if somebody is missing :)
________________________________________________________________________________
Microsoft Teams Need help?
Join the meeting now
Meeting ID: 246 246 238 55
Passcode: LkmwY8
________________________________
Dial-in by phone
+1 323-849-4874,,536269082# United States, Los Angeles
Find a local number
Phone conference ID: 536 269 082#
For organizers: Meeting options | Reset dial-in PIN
________________________________________________________________________________`},
	})
	if err != nil {
		t.Fatalf("convertEvent error: %v", err)
	}

	if event.Meeting.Service != "Microsoft Teams Meeting" {
		t.Fatalf("unexpected meeting service: %q", event.Meeting.Service)
	}
	if event.Meeting.ID != "246 246 238 55" {
		t.Fatalf("unexpected meeting ID: %q", event.Meeting.ID)
	}
	if event.Meeting.Passcode != "LkmwY8" {
		t.Fatalf("unexpected passcode: %q", event.Meeting.Passcode)
	}
	if event.Meeting.DialIn != "+1 323-849-4874,,536269082# United States, Los Angeles" {
		t.Fatalf("unexpected dial-in: %q", event.Meeting.DialIn)
	}
	if event.Meeting.PhoneConferenceID != "536 269 082#" {
		t.Fatalf("unexpected phone conference ID: %q", event.Meeting.PhoneConferenceID)
	}
	if strings.Contains(event.Description, "Microsoft Teams") || strings.Contains(event.Description, "Meeting ID:") {
		t.Fatalf("expected Teams footer to be removed, got %q", event.Description)
	}
	if !strings.Contains(event.Description, "Adding meeting room for folks in Redmond") {
		t.Fatalf("expected real description content, got %q", event.Description)
	}
}
