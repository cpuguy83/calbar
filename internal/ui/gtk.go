//go:build !nogtk && cgo

package ui

import (
	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/links"
)

// GTK wraps the Popup to implement the UI interface.
type GTK struct {
	popup    *Popup
	onAction func(Action)
}

// NewGTK creates a new GTK UI backend.
func NewGTK(cfg Config) *GTK {
	return &GTK{
		popup: NewPopup(cfg.TimeRange, cfg.NoAutoDismiss),
	}
}

// GTKAvailable returns true if GTK is available.
// For puregotk builds, GTK is always "available" at the code level.
// If GTK libraries aren't installed, the program will panic on startup
// with a clear error message from puregotk about missing libraries.
// Use the 'nogtk' build tag to build without GTK support for systems
// that don't have GTK4 installed.
func GTKAvailable() bool {
	return true
}

// Init initializes the GTK UI.
func (g *GTK) Init() error {
	g.popup.Init()
	g.popup.OnJoin(func(url string) {
		if g.onAction != nil {
			g.onAction(Action{Type: ActionOpenURL, URL: url})
		} else {
			links.Open(url)
		}
	})
	return nil
}

// Show displays the popup.
func (g *GTK) Show() {
	g.popup.Show()
}

// Hide hides the popup.
func (g *GTK) Hide() {
	g.popup.Hide()
}

// Toggle shows or hides the popup.
func (g *GTK) Toggle() {
	g.popup.Toggle()
}

// SetEvents updates the event list.
func (g *GTK) SetEvents(events []calendar.Event) {
	g.popup.SetEvents(events)
}

// SetStale marks the data as potentially stale.
func (g *GTK) SetStale(stale bool) {
	g.popup.SetStale(stale)
}

// OnAction sets the callback for user actions.
func (g *GTK) OnAction(fn func(Action)) {
	g.onAction = fn
}
