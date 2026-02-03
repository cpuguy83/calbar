//go:build nogtk || !cgo

package ui

import (
	"github.com/cpuguy83/calbar/internal/calendar"
)

// GTK is a stub when GTK is not available.
type GTK struct{}

// NewGTK returns nil when GTK is not available.
func NewGTK(cfg Config) *GTK {
	return nil
}

// GTKAvailable returns false when GTK is not available.
func GTKAvailable() bool {
	return false
}

// Init is a no-op stub.
func (g *GTK) Init() error {
	return nil
}

// Show is a no-op stub.
func (g *GTK) Show() {}

// Hide is a no-op stub.
func (g *GTK) Hide() {}

// Toggle is a no-op stub.
func (g *GTK) Toggle() {}

// SetEvents is a no-op stub.
func (g *GTK) SetEvents(events []calendar.Event) {}

// SetStale is a no-op stub.
func (g *GTK) SetStale(stale bool) {}

// OnAction is a no-op stub.
func (g *GTK) OnAction(fn func(Action)) {}
