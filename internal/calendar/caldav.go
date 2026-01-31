package calendar

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

// CalDAVSource fetches events from a CalDAV server.
type CalDAVSource struct {
	name      string
	url       string
	username  string
	password  string
	calendars []string // Optional: specific calendars to sync
}

// NewCalDAVSource creates a new CalDAV calendar source.
func NewCalDAVSource(name, url, username, password string, calendars []string) *CalDAVSource {
	return &CalDAVSource{
		name:      name,
		url:       url,
		username:  username,
		password:  password,
		calendars: calendars,
	}
}

// iCloudCalDAVURL is the base URL for iCloud CalDAV.
const iCloudCalDAVURL = "https://caldav.icloud.com"

// NewICloudSource creates a new iCloud calendar source.
// iCloud uses CalDAV with a specific server URL.
func NewICloudSource(name, username, password string, calendars []string) *CalDAVSource {
	return &CalDAVSource{
		name:      name,
		url:       iCloudCalDAVURL,
		username:  username,
		password:  password,
		calendars: calendars,
	}
}

// Name returns the display name of this calendar source.
func (s *CalDAVSource) Name() string {
	return s.name
}

// Fetch retrieves events from the CalDAV server.
func (s *CalDAVSource) Fetch(ctx context.Context) ([]Event, error) {
	// Create HTTP client with basic auth
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &basicAuthTransport{
			username: s.username,
			password: s.password,
			base:     http.DefaultTransport,
		},
	}

	// Create CalDAV client
	client, err := caldav.NewClient(httpClient, s.url)
	if err != nil {
		return nil, fmt.Errorf("create caldav client: %w", err)
	}

	// Find the user's calendar home
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("find principal: %w", err)
	}

	homeSet, err := client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("find calendar home: %w", err)
	}

	// Find all calendars
	cals, err := client.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("find calendars: %w", err)
	}

	var allEvents []Event

	// Filter calendars if specific ones requested
	for _, cal := range cals {
		if len(s.calendars) > 0 && !s.shouldSyncCalendar(cal.Name) {
			continue
		}

		events, err := s.fetchCalendarEvents(ctx, client, cal)
		if err != nil {
			// Log but continue with other calendars
			continue
		}

		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}

// shouldSyncCalendar checks if a calendar should be synced based on config.
func (s *CalDAVSource) shouldSyncCalendar(name string) bool {
	for _, c := range s.calendars {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

// fetchCalendarEvents fetches events from a single calendar.
func (s *CalDAVSource) fetchCalendarEvents(ctx context.Context, client *caldav.Client, cal caldav.Calendar) ([]Event, error) {
	// Query for events in a reasonable time range
	now := time.Now()
	start := now.Add(-7 * 24 * time.Hour) // 1 week ago
	end := now.Add(90 * 24 * time.Hour)   // 90 days ahead

	query := &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name: "VCALENDAR",
			Comps: []caldav.CalendarCompRequest{{
				Name: "VEVENT",
				Props: []string{
					"SUMMARY",
					"DTSTART",
					"DTEND",
					"DURATION",
					"UID",
					"DESCRIPTION",
					"LOCATION",
					"URL",
					"ORGANIZER",
				},
			}},
		},
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{{
				Name:  "VEVENT",
				Start: start,
				End:   end,
			}},
		},
	}

	objects, err := client.QueryCalendar(ctx, cal.Path, query)
	if err != nil {
		return nil, fmt.Errorf("query calendar %s: %w", cal.Name, err)
	}

	var events []Event
	for _, obj := range objects {
		if obj.Data == nil {
			continue
		}

		parsed, err := s.parseCalendarObject(obj.Data, cal.Name)
		if err != nil {
			continue
		}

		events = append(events, parsed...)
	}

	return events, nil
}

// parseCalendarObject parses a CalDAV calendar object into events.
func (s *CalDAVSource) parseCalendarObject(data *ics.Calendar, calName string) ([]Event, error) {
	var events []Event

	for _, comp := range data.Children {
		if comp.Name != ics.CompEvent {
			continue
		}

		event, err := s.parseEventComponent(comp, calName)
		if err != nil {
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

// parseEventComponent converts an ICS VEVENT to our Event type.
func (s *CalDAVSource) parseEventComponent(comp *ics.Component, calName string) (Event, error) {
	event := Event{
		Source: fmt.Sprintf("%s/%s", s.name, calName),
	}

	// UID
	if prop := comp.Props.Get(ics.PropUID); prop != nil {
		event.UID = prop.Value
	}

	// Summary (title)
	if prop := comp.Props.Get(ics.PropSummary); prop != nil {
		event.Summary = prop.Value
	}

	// Description
	if prop := comp.Props.Get(ics.PropDescription); prop != nil {
		event.Description = prop.Value
	}

	// Location
	if prop := comp.Props.Get(ics.PropLocation); prop != nil {
		event.Location = prop.Value
	}

	// URL
	if prop := comp.Props.Get(ics.PropURL); prop != nil {
		event.URL = prop.Value
	}

	// Organizer
	if prop := comp.Props.Get(ics.PropOrganizer); prop != nil {
		event.Organizer = prop.Value
		if len(event.Organizer) > 7 && event.Organizer[:7] == "mailto:" {
			event.Organizer = event.Organizer[7:]
		}
	}

	// Start time
	if prop := comp.Props.Get(ics.PropDateTimeStart); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			// Try as date-only (all-day event)
			t, err = time.ParseInLocation("20060102", prop.Value, time.Local)
			if err != nil {
				return event, fmt.Errorf("parse start time: %w", err)
			}
			event.AllDay = true
		}
		event.Start = t
	}

	// End time
	if prop := comp.Props.Get(ics.PropDateTimeEnd); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			// Try as date-only
			t, _ = time.ParseInLocation("20060102", prop.Value, time.Local)
		}
		event.End = t
	} else {
		// Default duration
		event.End = event.Start.Add(time.Hour)
	}

	return event, nil
}

// basicAuthTransport adds basic auth to HTTP requests.
type basicAuthTransport struct {
	username string
	password string
	base     http.RoundTripper
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.username, t.password)
	return t.base.RoundTrip(req)
}

// Ensure CalDAVSource implements Source interface.
var _ Source = (*CalDAVSource)(nil)
