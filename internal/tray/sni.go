// Package tray provides system tray integration using StatusNotifierItem (SNI).
package tray

import (
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	// D-Bus interface names
	sniInterface     = "org.kde.StatusNotifierItem"
	sniPath          = "/StatusNotifierItem"
	menuInterface    = "com.canonical.dbusmenu"
	menuPath         = sniPath + "/Menu"
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
	menu    dbusMenu

	// Callbacks
	onActivate   func() // Called when tray icon is clicked
	onOpenConfig func()
	onQuit       func()

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
	t.menu.tray = t

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
	if err := t.conn.Export(&t.menu, menuPath, menuInterface); err != nil {
		return fmt.Errorf("export menu interface: %w", err)
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
			"Menu":          {Value: dbus.ObjectPath(menuPath), Writable: false, Emit: prop.EmitFalse},
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
		Name:     sniPath,
		Children: []introspect.Node{{Name: "Menu"}},
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
	menuNode := &introspect.Node{
		Name: menuPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{Name: menuInterface, Methods: menuMethods, Signals: menuSignals},
		},
	}
	if err := t.conn.Export(introspect.NewIntrospectable(menuNode), menuPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("export menu introspection: %w", err)
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

// OnOpenConfig sets the callback for the tray menu's Open Config action.
func (t *Tray) OnOpenConfig(fn func()) {
	t.onOpenConfig = fn
}

// OnQuit sets the callback for the tray menu's Exit action.
func (t *Tray) OnQuit(fn func()) {
	t.onQuit = fn
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

type menuItem struct {
	ID         int32
	Label      string
	Activation func()
}

type menuItemProps map[string]dbus.Variant

type menuLayout struct {
	ID         int32
	Properties menuItemProps
	Children   []dbus.Variant
}

type menuProperty struct {
	ID         int32
	Properties menuItemProps
}

type dbusMenu struct {
	tray     *Tray
	revision uint32
}

func (m *dbusMenu) items() []menuItem {
	return []menuItem{
		{ID: 1, Label: "Open Config", Activation: m.tray.onOpenConfig},
		{ID: 2, Label: "Exit", Activation: m.tray.onQuit},
	}
}

func (m *dbusMenu) GetLayout(parentID int32, recursionDepth int32, propertyNames []string) (uint32, menuLayout, *dbus.Error) {
	children := make([]dbus.Variant, 0, len(m.items()))
	for _, item := range m.items() {
		if parentID != 0 && parentID != item.ID {
			continue
		}
		children = append(children, dbus.MakeVariant(menuLayout{ID: item.ID, Properties: m.propertiesFor(item, propertyNames)}))
	}
	if parentID != 0 {
		for _, child := range children {
			layout, ok := child.Value().(menuLayout)
			if !ok {
				continue
			}
			if layout.ID == parentID {
				return m.revision, layout, nil
			}
		}
		return m.revision, menuLayout{}, dbus.MakeFailedError(fmt.Errorf("unknown menu item %d", parentID))
	}
	return m.revision, menuLayout{ID: 0, Properties: menuItemProps{}, Children: children}, nil
}

func (m *dbusMenu) GetGroupProperties(ids []int32, propertyNames []string) ([]menuProperty, *dbus.Error) {
	items := m.items()
	result := make([]menuProperty, 0, len(ids))
	for _, id := range ids {
		for _, item := range items {
			if item.ID != id {
				continue
			}
			result = append(result, menuProperty{ID: item.ID, Properties: m.propertiesFor(item, propertyNames)})
			break
		}
	}
	return result, nil
}

func (m *dbusMenu) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}
	for _, item := range m.items() {
		if item.ID != id {
			continue
		}
		if item.Activation != nil {
			go item.Activation()
		}
		return nil
	}
	return dbus.MakeFailedError(fmt.Errorf("unknown menu item %d", id))
}

func (m *dbusMenu) AboutToShow(id int32) (bool, *dbus.Error) {
	return false, nil
}

func (m *dbusMenu) propertiesFor(item menuItem, propertyNames []string) menuItemProps {
	props := menuItemProps{
		"label":   dbus.MakeVariant(item.Label),
		"enabled": dbus.MakeVariant(item.Activation != nil),
		"visible": dbus.MakeVariant(true),
	}
	if len(propertyNames) == 0 {
		return props
	}
	filtered := make(menuItemProps, len(propertyNames))
	for _, name := range propertyNames {
		if value, ok := props[name]; ok {
			filtered[name] = value
		}
	}
	return filtered
}

func (m *dbusMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	for _, item := range m.items() {
		if item.ID != id {
			continue
		}
		props := m.propertiesFor(item, []string{name})
		if value, ok := props[name]; ok {
			return value, nil
		}
		break
	}
	return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown menu property %q for item %d", name, id))
}

func (m *dbusMenu) EventGroup(events []struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}) ([]int32, []int32, *dbus.Error) {
	processed := make([]int32, 0, len(events))
	for _, event := range events {
		if err := m.Event(event.ID, event.EventID, event.Data, event.Timestamp); err != nil {
			return processed, nil, err
		}
		processed = append(processed, event.ID)
	}
	return processed, nil, nil
}

func (m *dbusMenu) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	updated := make([]int32, 0, len(ids))
	for _, id := range ids {
		if id == 0 || slices.ContainsFunc(m.items(), func(item menuItem) bool { return item.ID == id }) {
			updated = append(updated, id)
		}
	}
	return updated, nil, nil
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

var menuMethods = []introspect.Method{
	{
		Name: "GetLayout",
		Args: []introspect.Arg{
			{Name: "parentId", Type: "i", Direction: "in"},
			{Name: "recursionDepth", Type: "i", Direction: "in"},
			{Name: "propertyNames", Type: "as", Direction: "in"},
			{Name: "revision", Type: "u", Direction: "out"},
			{Name: "layout", Type: "(ia{sv}av)", Direction: "out"},
		},
	},
	{
		Name: "GetGroupProperties",
		Args: []introspect.Arg{
			{Name: "ids", Type: "ai", Direction: "in"},
			{Name: "propertyNames", Type: "as", Direction: "in"},
			{Name: "properties", Type: "a(ia{sv})", Direction: "out"},
		},
	},
	{
		Name: "GetProperty",
		Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "name", Type: "s", Direction: "in"},
			{Name: "value", Type: "v", Direction: "out"},
		},
	},
	{
		Name: "Event",
		Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "eventId", Type: "s", Direction: "in"},
			{Name: "data", Type: "v", Direction: "in"},
			{Name: "timestamp", Type: "u", Direction: "in"},
		},
	},
	{
		Name: "EventGroup",
		Args: []introspect.Arg{
			{Name: "events", Type: "a(isvu)", Direction: "in"},
			{Name: "idsNeedingRefresh", Type: "ai", Direction: "out"},
			{Name: "idErrors", Type: "ai", Direction: "out"},
		},
	},
	{
		Name: "AboutToShow",
		Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "needUpdate", Type: "b", Direction: "out"},
		},
	},
	{
		Name: "AboutToShowGroup",
		Args: []introspect.Arg{
			{Name: "ids", Type: "ai", Direction: "in"},
			{Name: "updatesNeeded", Type: "ai", Direction: "out"},
			{Name: "idErrors", Type: "ai", Direction: "out"},
		},
	},
}

var menuSignals = []introspect.Signal{
	{Name: "LayoutUpdated", Args: []introspect.Arg{{Name: "revision", Type: "u"}, {Name: "parent", Type: "i"}}},
}
