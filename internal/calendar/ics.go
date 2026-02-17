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
	end      time.Time // end of time range for filtering
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
func (s *ICSSource) Fetch(ctx context.Context, end time.Time) ([]Event, error) {
	s.end = end

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
	now := time.Now()

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

			parsed, err := s.parseEvent(comp)
			if err != nil {
				// Skip events we can't parse
				continue
			}

			// Filter events to the configured time range (now to end)
			for _, event := range parsed {
				// Include events that end after now and start before end
				if event.End.After(now) && event.Start.Before(s.end) {
					events = append(events, event)
				}
			}
		}
	}

	return events, nil
}

// parseEvent converts an ICS VEVENT component to our Event type.
// For recurring events, it expands occurrences within the configured time range.
func (s *ICSSource) parseEvent(comp *ics.Component) ([]Event, error) {
	base := Event{
		Source: s.name,
	}

	// UID
	if prop := comp.Props.Get(ics.PropUID); prop != nil {
		base.UID = prop.Value
	}

	// Summary (title)
	if prop := comp.Props.Get(ics.PropSummary); prop != nil {
		base.Summary = prop.Value
	}

	// Description
	if prop := comp.Props.Get(ics.PropDescription); prop != nil {
		base.Description = prop.Value
	}

	// Location
	if prop := comp.Props.Get(ics.PropLocation); prop != nil {
		base.Location = prop.Value
	}

	// URL
	if prop := comp.Props.Get(ics.PropURL); prop != nil {
		base.URL = prop.Value
	}

	// Organizer
	if prop := comp.Props.Get(ics.PropOrganizer); prop != nil {
		base.Organizer = prop.Value
		// Strip "mailto:" prefix if present
		if len(base.Organizer) > 7 && base.Organizer[:7] == "mailto:" {
			base.Organizer = base.Organizer[7:]
		}
	}

	// Start time
	var startTime time.Time
	var isAllDay bool
	if prop := comp.Props.Get(ics.PropDateTimeStart); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			// Try parsing as local datetime without timezone (floating time)
			t, err = parseDateTime(prop.Value)
			if err != nil {
				// Try as date-only (all-day event)
				t, err = parseDateOnly(prop.Value)
				if err != nil {
					return nil, fmt.Errorf("parse start time: %w", err)
				}
				isAllDay = true
			}
		}
		startTime = t
	}

	// End time / duration
	var duration time.Duration
	if prop := comp.Props.Get(ics.PropDateTimeEnd); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			// Try parsing as local datetime without timezone (floating time)
			t, err = parseDateTime(prop.Value)
			if err != nil {
				// Try as date-only (all-day event)
				t, err = parseDateOnly(prop.Value)
				if err != nil {
					return nil, fmt.Errorf("parse end time: %w", err)
				}
			}
		}
		duration = t.Sub(startTime)
	} else if prop := comp.Props.Get(ics.PropDuration); prop != nil {
		// TODO: Parse duration properly
		duration = time.Hour
	} else {
		// Default to 1 hour duration
		duration = time.Hour
	}

	// Check for recurrence rule
	rset, err := comp.RecurrenceSet(time.Local)
	if err != nil {
		return nil, fmt.Errorf("parse recurrence: %w", err)
	}

	if rset == nil {
		// Non-recurring event
		base.Start = startTime
		base.End = startTime.Add(duration)
		base.AllDay = isAllDay || isEffectivelyAllDay(base.Start, base.End)
		return []Event{base}, nil
	}

	// Recurring event - expand occurrences
	// Look back by duration to catch events that have started but haven't ended yet
	now := time.Now()
	rangeStart := now.Add(-duration)
	rangeEnd := s.end

	occurrences := rset.Between(rangeStart, rangeEnd, true)

	var events []Event
	for _, occ := range occurrences {
		event := base // Copy base event
		event.Start = occ
		event.End = occ.Add(duration)
		event.AllDay = isAllDay || isEffectivelyAllDay(event.Start, event.End)
		// Make UID unique per occurrence
		event.UID = fmt.Sprintf("%s_%d", base.UID, occ.Unix())
		events = append(events, event)
	}

	return events, nil
}

// parseDateOnly parses a date-only value (YYYYMMDD format).
func parseDateOnly(s string) (time.Time, error) {
	return time.ParseInLocation("20060102", s, time.Local)
}

// parseDateTime parses a datetime value without timezone (YYYYMMDDTHHmmss format).
// This handles "floating time" values that are neither UTC nor have a TZID.
func parseDateTime(s string) (time.Time, error) {
	return time.ParseInLocation("20060102T150405", s, time.Local)
}
