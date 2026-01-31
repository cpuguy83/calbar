package calendar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	ics "github.com/emersion/go-ical"
)

// ICSSource fetches events from an ICS/iCal URL.
type ICSSource struct {
	name     string
	url      string
	username string
	password string
	client   *http.Client
}

// NewICSSource creates a new ICS calendar source.
func NewICSSource(name, url, username, password string) *ICSSource {
	return &ICSSource{
		name:     name,
		url:      url,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the display name of this calendar source.
func (s *ICSSource) Name() string {
	return s.name
}

// Fetch retrieves events from the ICS feed.
func (s *ICSSource) Fetch(ctx context.Context) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add basic auth if credentials provided
	if s.username != "" && s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ICS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch ICS: status %d", resp.StatusCode)
	}

	return s.parseICS(resp.Body)
}

// parseICS parses an ICS file and returns events.
func (s *ICSSource) parseICS(r io.Reader) ([]Event, error) {
	dec := ics.NewDecoder(r)

	var events []Event

	for {
		cal, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode ICS: %w", err)
		}

		for _, comp := range cal.Children {
			if comp.Name != ics.CompEvent {
				continue
			}

			event, err := s.parseEvent(comp)
			if err != nil {
				// Skip events we can't parse
				continue
			}

			events = append(events, event)
		}
	}

	return events, nil
}

// parseEvent converts an ICS VEVENT component to our Event type.
func (s *ICSSource) parseEvent(comp *ics.Component) (Event, error) {
	event := Event{
		Source: s.name,
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
		// Strip "mailto:" prefix if present
		if len(event.Organizer) > 7 && event.Organizer[:7] == "mailto:" {
			event.Organizer = event.Organizer[7:]
		}
	}

	// Start time
	if prop := comp.Props.Get(ics.PropDateTimeStart); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			// Try as date-only (all-day event)
			t, err = parseDateOnly(prop.Value)
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
			// Try as date-only (all-day event)
			t, err = parseDateOnly(prop.Value)
			if err != nil {
				return event, fmt.Errorf("parse end time: %w", err)
			}
		}
		event.End = t
	} else if prop := comp.Props.Get(ics.PropDuration); prop != nil {
		// Duration instead of end time
		// TODO: Parse duration properly
		event.End = event.Start.Add(time.Hour)
	} else {
		// Default to 1 hour duration
		event.End = event.Start.Add(time.Hour)
	}

	return event, nil
}

// parseDateOnly parses a date-only value (YYYYMMDD format).
func parseDateOnly(s string) (time.Time, error) {
	return time.ParseInLocation("20060102", s, time.Local)
}
