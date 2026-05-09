//go:build darwin

// Package tray provides system tray integration.
package tray

import "github.com/cpuguy83/calbar/internal/macos"

// State represents the tray icon state.
type State int

const (
	StateNormal State = iota
	StateImminent
	StateStale
)

// Tray manages the native macOS status item through the Swift helper.
type Tray struct {
	frontend *macos.Frontend
}

// New creates a new system tray icon.
func New() (*Tray, error) {
	return &Tray{frontend: macos.Shared()}, nil
}

// Start starts the native macOS helper.
func (t *Tray) Start() error {
	return t.frontend.Start()
}

// Stop removes the tray icon.
func (t *Tray) Stop() error {
	return t.frontend.Stop()
}

// SetState updates the tray icon state.
func (t *Tray) SetState(state State) {
	_ = t.frontend.Send(macos.Command{Type: "set_tray_state", State: stateName(state)})
}

// SetTooltip updates the tooltip text.
func (t *Tray) SetTooltip(text string) {
	_ = t.frontend.Send(macos.Command{Type: "set_tooltip", Tooltip: text})
}

// OnActivate sets the callback for when the tray icon is clicked.
func (t *Tray) OnActivate(fn func()) {
	t.frontend.OnActivate(fn)
}

// OnCopyConfigPath sets the callback for the tray menu's Copy Config Path action.
func (t *Tray) OnCopyConfigPath(fn func()) {
	t.frontend.OnCopyConfigPath(fn)
}

// OnQuit sets the callback for the tray menu's Exit action.
func (t *Tray) OnQuit(fn func()) {
	t.frontend.OnQuit(fn)
}

func stateName(state State) string {
	switch state {
	case StateImminent:
		return "imminent"
	case StateStale:
		return "stale"
	default:
		return "normal"
	}
}
