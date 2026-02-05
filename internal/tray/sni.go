// Package tray provides system tray integration using StatusNotifierItem (SNI).
package tray

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	// D-Bus interface names
	sniInterface     = "org.kde.StatusNotifierItem"
	sniPath          = "/StatusNotifierItem"
	watcherInterface = "org.kde.StatusNotifierWatcher"
	watcherPath      = "/StatusNotifierWatcher"
	watcherBusName   = "org.kde.StatusNotifierWatcher"

	// Icon names (using freedesktop standard icons)
	iconNormal   = "x-office-calendar"
	iconImminent = "appointment-soon"
	iconStale    = "dialog-warning"
)

// State represents the tray icon state.
type State int

const (
	StateNormal State = iota
	StateImminent
	StateStale
)

// Tray manages the system tray icon via StatusNotifierItem.
type Tray struct {
	conn    *dbus.Conn
	busName string
	props   *prop.Properties

	state   State
	tooltip toolTip

	// Callbacks
	onActivate func() // Called when tray icon is clicked

	// For clean shutdown of watcher goroutine
	stopCh chan struct{}
}

// New creates a new system tray icon.
func New() (*Tray, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	t := &Tray{
		conn:   conn,
		state:  StateNormal,
		stopCh: make(chan struct{}),
		tooltip: toolTip{
			Title: "CalBar",
			Body:  "CalBar",
		},
	}

	return t, nil
}

// Start registers the tray icon with the StatusNotifierWatcher.
func (t *Tray) Start() error {
	// Request a unique bus name using process ID
	busName := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := t.conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		// Fall back to a simpler name
		busName = "org.kde.StatusNotifierItem-calbar"
		reply, err = t.conn.RequestName(busName, dbus.NameFlagDoNotQueue)
		if err != nil {
			return fmt.Errorf("request bus name: %w", err)
		}
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("bus name already taken")
	}

	t.busName = busName

	// Export the SNI object (methods)
	if err := t.conn.Export(t, sniPath, sniInterface); err != nil {
		return fmt.Errorf("export SNI interface: %w", err)
	}

	// Setup properties using godbus prop package
	propsSpec := prop.Map{
		sniInterface: {
			"Category":      {Value: "ApplicationStatus", Writable: false, Emit: prop.EmitFalse},
			"Id":            {Value: "calbar", Writable: false, Emit: prop.EmitFalse},
			"Title":         {Value: "CalBar", Writable: false, Emit: prop.EmitFalse},
			"Status":        {Value: "Active", Writable: false, Emit: prop.EmitTrue},
			"IconName":      {Value: "", Writable: false, Emit: prop.EmitTrue},
			"IconPixmap":    {Value: t.getIconPixmap(), Writable: false, Emit: prop.EmitTrue},
			"IconThemePath": {Value: "", Writable: false, Emit: prop.EmitFalse},
			"Menu":          {Value: dbus.ObjectPath("/NO_DBUSMENU"), Writable: false, Emit: prop.EmitFalse},
			"ItemIsMenu":    {Value: false, Writable: false, Emit: prop.EmitFalse},
			"ToolTip":       {Value: t.getToolTip(), Writable: false, Emit: prop.EmitTrue},
		},
	}

	props, err := prop.Export(t.conn, sniPath, propsSpec)
	if err != nil {
		return fmt.Errorf("export properties: %w", err)
	}
	t.props = props

	// Export introspection data
	node := &introspect.Node{
		Name: sniPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:    sniInterface,
				Methods: sniMethods,
				Signals: sniSignals,
			},
		},
	}
	if err := t.conn.Export(introspect.NewIntrospectable(node), sniPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export introspection: %w", err)
	}

	// Initial registration with the watcher
	t.registerWithWatcher()

	// Watch for StatusNotifierWatcher restarts (e.g., when waybar restarts)
	go t.handleWatcherSignals()

	slog.Info("tray icon registered", "bus_name", t.busName, "connection", t.conn.Names()[0])
	return nil
}

// registerWithWatcher registers this tray icon with the StatusNotifierWatcher.
// This is called on startup and whenever the watcher service restarts.
func (t *Tray) registerWithWatcher() {
	uniqueName := t.conn.Names()[0]
	watcher := t.conn.Object(watcherBusName, watcherPath)
	call := watcher.Call(watcherInterface+".RegisterStatusNotifierItem", 0, uniqueName)
	if call.Err != nil {
		slog.Warn("failed to register with StatusNotifierWatcher", "error", call.Err)
		// Continue anyway - some environments don't have a watcher
	} else {
		slog.Debug("registered with StatusNotifierWatcher", "connection", uniqueName)
	}
}

// handleWatcherSignals listens for D-Bus signals indicating the StatusNotifierWatcher
// service has restarted (e.g., when waybar restarts) and re-registers our tray icon.
func (t *Tray) handleWatcherSignals() {
	// Subscribe to NameOwnerChanged signals for the watcher bus name
	matchRule := fmt.Sprintf(
		"type='signal',interface='org.freedesktop.DBus',member='NameOwnerChanged',arg0='%s'",
		watcherBusName,
	)
	if err := t.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		slog.Warn("failed to add D-Bus match rule for watcher monitoring", "error", err)
		return
	}

	// Channel for D-Bus signals - size 1 acts as a coalescing buffer
	// If multiple signals arrive while we're processing, we only need to
	// re-register once, so dropping intermediate signals is fine
	sigCh := make(chan *dbus.Signal, 1)
	t.conn.Signal(sigCh)

	defer t.conn.RemoveSignal(sigCh)

	for {
		select {
		case <-t.stopCh:
			return
		case sig, ok := <-sigCh:
			if !ok {
				return
			}
			// NameOwnerChanged has args: (name string, old_owner string, new_owner string)
			if sig.Name != "org.freedesktop.DBus.NameOwnerChanged" {
				continue
			}
			if len(sig.Body) < 3 {
				continue
			}
			name, ok := sig.Body[0].(string)
			if !ok || name != watcherBusName {
				continue
			}
			newOwner, ok := sig.Body[2].(string)
			if !ok {
				continue
			}
			// If the watcher has a new owner (non-empty), re-register
			if newOwner != "" {
				slog.Info("StatusNotifierWatcher restarted, re-registering tray icon")
				t.registerWithWatcher()
			}
		}
	}
}

// Stop removes the tray icon.
func (t *Tray) Stop() error {
	close(t.stopCh)
	return t.conn.Close()
}

// SetState updates the tray icon state.
func (t *Tray) SetState(state State) {
	t.state = state

	// Update property and emit signal
	if t.props != nil {
		t.props.SetMust(sniInterface, "IconPixmap", t.getIconPixmap())
	}
	t.conn.Emit(sniPath, sniInterface+".NewIcon")
}

// SetTooltip updates the tooltip text.
func (t *Tray) SetTooltip(text string) {
	t.tooltip.Body = text

	// Update property and emit signal
	if t.props != nil {
		t.props.SetMust(sniInterface, "ToolTip", t.tooltip)
	}
	t.conn.Emit(sniPath, sniInterface+".NewToolTip")
}

// OnActivate sets the callback for when the tray icon is clicked.
func (t *Tray) OnActivate(fn func()) {
	t.onActivate = fn
}

// getToolTip returns the current tooltip struct.
func (t *Tray) getToolTip() toolTip {
	return t.tooltip
}

// iconData represents a single icon in the pixmap array.
type iconData struct {
	Width  int32
	Height int32
	Data   []byte
}

// toolTip represents the StatusNotifierItem tooltip struct (sa(iiay)ss).
type toolTip struct {
	IconName   string     // Icon name (empty to use pixmap)
	IconPixmap []iconData // Icon pixmap (can be empty)
	Title      string     // Tooltip title
	Body       string     // Tooltip body/description
}

// getIconPixmap returns the current icon pixmap.
func (t *Tray) getIconPixmap() []iconData {
	var icon []byte
	switch t.state {
	case StateImminent:
		icon = iconImminentPixmap
	case StateStale:
		icon = iconStalePixmap
	default:
		icon = iconNormalPixmap
	}

	return []iconData{
		{Width: 22, Height: 22, Data: icon},
	}
}

// SNI D-Bus method implementations

// Activate is called when the user clicks the tray icon (primary action).
func (t *Tray) Activate(x, y int32) *dbus.Error {
	slog.Debug("tray activated", "x", x, "y", y)
	if t.onActivate != nil {
		go t.onActivate()
	}
	return nil
}

// SecondaryActivate is called on middle-click.
func (t *Tray) SecondaryActivate(x, y int32) *dbus.Error {
	slog.Debug("tray secondary activated", "x", x, "y", y)
	return nil
}

// Scroll is called when the user scrolls on the tray icon.
func (t *Tray) Scroll(delta int32, orientation string) *dbus.Error {
	slog.Debug("tray scroll", "delta", delta, "orientation", orientation)
	return nil
}

// ContextMenu is called to show a context menu (right-click).
func (t *Tray) ContextMenu(x, y int32) *dbus.Error {
	slog.Debug("tray context menu", "x", x, "y", y)
	return nil
}

// 22x22 calendar icon in ARGB format (network byte order: ARGB)
var iconNormalPixmap = generateCalendarIcon(0xFF5294E2)   // Blue header (Arc-style blue)
var iconImminentPixmap = generateCalendarIcon(0xFFF27835) // Orange header
var iconStalePixmap = generateCalendarIcon(0xFFCC575D)    // Red header

func generateCalendarIcon(headerColor uint32) []byte {
	const size = 22
	pixels := make([]byte, size*size*4)

	// Helper to set pixel (ARGB format, network byte order)
	setPixel := func(x, y int, argb uint32) {
		if x < 0 || x >= size || y < 0 || y >= size {
			return
		}
		i := (y*size + x) * 4
		pixels[i] = byte(argb >> 24)   // A
		pixels[i+1] = byte(argb >> 16) // R
		pixels[i+2] = byte(argb >> 8)  // G
		pixels[i+3] = byte(argb)       // B
	}

	// Draw filled rectangle
	fillRect := func(x1, y1, x2, y2 int, argb uint32) {
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				setPixel(x, y, argb)
			}
		}
	}

	// Colors
	transparent := uint32(0x00000000)
	bodyColor := uint32(0xFFF5F5F5)   // Light gray body
	borderColor := uint32(0xFF3D3D3D) // Dark border
	ringColor := uint32(0xFF4A4A4A)   // Calendar rings
	dotColor := uint32(0xFF404040)    // Date dots

	// Clear background
	fillRect(0, 0, size-1, size-1, transparent)

	// Calendar body with rounded corners (main shape)
	// Body area: x=2-19, y=4-19
	fillRect(3, 5, 18, 18, bodyColor)

	// Rounded corners for body
	setPixel(2, 6, bodyColor)
	setPixel(2, 17, bodyColor)
	setPixel(19, 6, bodyColor)
	setPixel(19, 17, bodyColor)

	// Top edge (below header)
	fillRect(3, 4, 18, 4, bodyColor)

	// Header bar with rounded top corners
	fillRect(3, 4, 18, 7, headerColor)
	setPixel(2, 5, headerColor)
	setPixel(2, 6, headerColor)
	setPixel(2, 7, headerColor)
	setPixel(19, 5, headerColor)
	setPixel(19, 6, headerColor)
	setPixel(19, 7, headerColor)

	// Border - left edge
	for y := 6; y <= 17; y++ {
		setPixel(2, y, borderColor)
	}
	// Border - right edge
	for y := 6; y <= 17; y++ {
		setPixel(19, y, borderColor)
	}
	// Border - bottom edge
	for x := 3; x <= 18; x++ {
		setPixel(x, 18, borderColor)
	}
	// Rounded bottom corners
	setPixel(3, 18, borderColor)
	setPixel(18, 18, borderColor)
	setPixel(2, 18, transparent)
	setPixel(19, 18, transparent)

	// Calendar rings/hooks at top
	for _, x := range []int{6, 10, 14} {
		setPixel(x, 2, ringColor)
		setPixel(x+1, 2, ringColor)
		setPixel(x, 3, ringColor)
		setPixel(x+1, 3, ringColor)
		setPixel(x, 4, ringColor)
		setPixel(x+1, 4, ringColor)
	}

	// Date grid dots (3x3 grid of dots representing dates)
	// Row 1: y=10
	for _, x := range []int{6, 10, 14} {
		fillRect(x, 10, x+1, 11, dotColor)
	}
	// Row 2: y=13
	for _, x := range []int{6, 10, 14} {
		fillRect(x, 13, x+1, 14, dotColor)
	}
	// Row 3: y=16 (only first two)
	for _, x := range []int{6, 10} {
		fillRect(x, 16, x+1, 17, dotColor)
	}

	return pixels
}

// D-Bus interface definitions for introspection
var sniMethods = []introspect.Method{
	{Name: "Activate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
	{Name: "SecondaryActivate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
	{Name: "Scroll", Args: []introspect.Arg{{Name: "delta", Type: "i", Direction: "in"}, {Name: "orientation", Type: "s", Direction: "in"}}},
	{Name: "ContextMenu", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
}

var sniSignals = []introspect.Signal{
	{Name: "NewIcon"},
	{Name: "NewToolTip"},
	{Name: "NewStatus", Args: []introspect.Arg{{Name: "status", Type: "s"}}},
}
