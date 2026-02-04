// Package calendar provides calendar source interfaces and event types.
package calendar

import (
	"context"
	"time"
)

// Event represents a calendar event.
type Event struct {
	// UID is the unique identifier for this event.
	UID string

	// Summary is the event title.
	Summary string

	// Description is the full event description/body.
	Description string

	// Location is the event location (may contain meeting URLs).
	Location string

	// Start is when the event begins.
	Start time.Time

	// End is when the event ends.
	End time.Time

	// AllDay indicates this is an all-day event.
	AllDay bool

	// Organizer is the email of the event organizer.
	Organizer string

	// Source is the name of the calendar source this event came from.
	Source string

	// URL is a URL associated with the event (if any).
	URL string
}

// Duration returns the duration of the event.
func (e *Event) Duration() time.Duration {
	return e.End.Sub(e.Start)
}

// IsOngoing returns true if the event is currently happening.
func (e *Event) IsOngoing(now time.Time) bool {
	return now.After(e.Start) && now.Before(e.End)
}

// IsUpcoming returns true if the event starts within the given duration.
func (e *Event) IsUpcoming(now time.Time, within time.Duration) bool {
	until := e.Start.Sub(now)
	return until > 0 && until <= within
}

// StartsIn returns how long until the event starts (negative if already started).
func (e *Event) StartsIn(now time.Time) time.Duration {
	return e.Start.Sub(now)
}

// Source is the interface that calendar sources must implement.
type Source interface {
	// Name returns the display name of this calendar source.
	Name() string

	// Fetch retrieves events from the calendar source.
	// Events should be fetched from now until the specified end time.
	Fetch(ctx context.Context, end time.Time) ([]Event, error)
}
