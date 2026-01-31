package calendar

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	ics "github.com/emersion/go-ical"
)

// Merge combines events from multiple sources into a single slice.
// Events are sorted by start time.
func Merge(eventSets ...[]Event) []Event {
	var all []Event
	for _, events := range eventSets {
		all = append(all, events...)
	}

	// Sort by start time
	sort.Slice(all, func(i, j int) bool {
		return all[i].Start.Before(all[j].Start)
	})

	return all
}

// WriteICS writes events to an ICS file atomically.
// It writes to a temp file first, then renames to the final path.
func WriteICS(path string, events []Event) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create the ICS content
	cal := ics.NewCalendar()
	cal.Props.SetText(ics.PropVersion, "2.0")
	cal.Props.SetText(ics.PropProductID, "-//CalBar//CalBar//EN")

	for _, event := range events {
		comp := ics.NewComponent(ics.CompEvent)

		comp.Props.SetText(ics.PropUID, event.UID)
		comp.Props.SetText(ics.PropSummary, event.Summary)

		// DTSTAMP is required by the ICS spec
		comp.Props.SetDateTime(ics.PropDateTimeStamp, time.Now())

		if event.Description != "" {
			comp.Props.SetText(ics.PropDescription, event.Description)
		}
		if event.Location != "" {
			comp.Props.SetText(ics.PropLocation, event.Location)
		}
		if event.URL != "" {
			comp.Props.SetText(ics.PropURL, event.URL)
		}
		if event.Organizer != "" {
			comp.Props.SetText(ics.PropOrganizer, "mailto:"+event.Organizer)
		}

		// Set start/end times
		if event.AllDay {
			comp.Props.SetDate(ics.PropDateTimeStart, event.Start)
			comp.Props.SetDate(ics.PropDateTimeEnd, event.End)
		} else {
			comp.Props.SetDateTime(ics.PropDateTimeStart, event.Start)
			comp.Props.SetDateTime(ics.PropDateTimeEnd, event.End)
		}

		// Add custom property for source
		comp.Props.SetText("X-CALBAR-SOURCE", event.Source)

		cal.Children = append(cal.Children, comp)
	}

	// Encode to buffer
	var buf bytes.Buffer
	enc := ics.NewEncoder(&buf)
	if err := enc.Encode(cal); err != nil {
		return fmt.Errorf("encode ICS: %w", err)
	}

	// Write to temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadICS reads events from an ICS file.
func ReadICS(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ICS file: %w", err)
	}
	defer f.Close()

	return ParseICS(f)
}

// ParseICS parses events from an ICS reader.
func ParseICS(r io.Reader) ([]Event, error) {
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

			event, err := parseEventComponent(comp)
			if err != nil {
				// Skip events we can't parse
				continue
			}

			events = append(events, event)
		}
	}

	// Sort by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	return events, nil
}

// parseEventComponent converts an ICS VEVENT component to our Event type.
func parseEventComponent(comp *ics.Component) (Event, error) {
	event := Event{}

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

	// Source (custom property)
	if prop := comp.Props.Get("X-CALBAR-SOURCE"); prop != nil {
		event.Source = prop.Value
	}

	// Start time
	if prop := comp.Props.Get(ics.PropDateTimeStart); prop != nil {
		t, err := prop.DateTime(time.Local)
		if err != nil {
			t, err = parseDateValue(prop.Value)
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
			t, err = parseDateValue(prop.Value)
			if err != nil {
				return event, fmt.Errorf("parse end time: %w", err)
			}
		}
		event.End = t
	} else {
		event.End = event.Start.Add(time.Hour)
	}

	return event, nil
}

// parseDateValue parses a date-only value (YYYYMMDD format).
func parseDateValue(s string) (time.Time, error) {
	return time.ParseInLocation("20060102", s, time.Local)
}
