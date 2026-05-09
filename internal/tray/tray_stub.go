//go:build !linux && !darwin

// Package tray provides system tray integration.
package tray

import "fmt"

// State represents the tray icon state.
type State int

const (
	StateNormal State = iota
	StateImminent
	StateStale
)

// Tray is a placeholder for platforms without an implementation yet.
type Tray struct {
	onActivate       func()
	onCopyConfigPath func()
	onQuit           func()
}

// New creates a new system tray icon.
func New() (*Tray, error) {
	return &Tray{}, nil
}

// Start registers the tray icon.
func (t *Tray) Start() error {
	return fmt.Errorf("system tray is not implemented on this platform")
}

// Stop removes the tray icon.
func (t *Tray) Stop() error {
	return nil
}

// SetState updates the tray icon state.
func (t *Tray) SetState(state State) {}

// SetTooltip updates the tooltip text.
func (t *Tray) SetTooltip(text string) {}

// OnActivate sets the callback for when the tray icon is clicked.
func (t *Tray) OnActivate(fn func()) {
	t.onActivate = fn
}

// OnCopyConfigPath sets the callback for the tray menu's Copy Config Path action.
func (t *Tray) OnCopyConfigPath(fn func()) {
	t.onCopyConfigPath = fn
}

// OnQuit sets the callback for the tray menu's Exit action.
func (t *Tray) OnQuit(fn func()) {
	t.onQuit = fn
}
