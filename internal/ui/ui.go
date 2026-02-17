// Package ui provides UI backends for calbar (GTK popup or dmenu-style launchers).
package ui

import (
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
)

// UI is the interface for displaying calendar events to the user.
type UI interface {
	// Init initializes the UI. Must be called before other methods.
	Init() error

	// Show displays the UI with current events.
	Show()

	// Hide hides the UI.
	Hide()

	// Toggle shows or hides the UI.
	Toggle()

	// SetEvents updates the event list.
	SetEvents(events []calendar.Event)

	// SetHiddenEvents updates the list of hidden events.
	SetHiddenEvents(events []calendar.Event)

	// SetStale marks the data as potentially stale.
	SetStale(stale bool)

	// OnAction sets the callback for when a user performs an action.
	OnAction(fn func(Action))

	// OnHide sets the callback for when the user hides an event.
	OnHide(fn func(uid string))

	// OnUnhide sets the callback for when the user unhides an event.
	OnUnhide(fn func(uid string))
}

// Action represents a user action from the UI.
type Action struct {
	Type ActionType
	URL  string // For ActionOpenURL
}

// ActionType identifies the type of action.
type ActionType int

const (
	// ActionOpenURL indicates the user wants to open a URL.
	ActionOpenURL ActionType = iota
)

// Config holds UI configuration.
type Config struct {
	TimeRange         time.Duration
	EventEndGrace     time.Duration // Keep events visible after they end
	HoverDismissDelay time.Duration // Delay before dismiss on pointer-leave (0 = never auto-dismiss)
}
