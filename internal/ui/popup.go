//go:build !nogtk && cgo

// Package ui provides the GTK4/libadwaita popup window for calbar.
package ui

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/gtk4layershell"
	"github.com/cpuguy83/calbar/internal/links"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/jwijenbergh/puregotk/v4/pango"
)

// stableCallback stores a callback function with sync.Once to ensure it's only
// initialized once. This is needed because puregotk caches callbacks by pointer
// address, so we need to return the same pointer each time to avoid exhausting
// purego's limited callback slots.
type stableCallback[T any] struct {
	once sync.Once
	fn   T
}

// get returns a pointer to the callback function, initializing it on first call.
func (s *stableCallback[T]) get(init func() T) *T {
	s.once.Do(func() {
		s.fn = init()
	})
	return &s.fn
}

// Popup is the main popup window showing upcoming events.
type Popup struct {
	window        *gtk.Window
	content       *gtk.Box
	listBox       *gtk.ListBox
	allDaySection *gtk.Box
	statusBar     *gtk.Label

	// Details panel
	stack        *gtk.Stack
	listView     *gtk.Box
	detailsView  *gtk.Box
	detailsEvent *calendar.Event

	mu            sync.RWMutex
	events        []calendar.Event
	timeRange     time.Duration
	eventEndGrace time.Duration
	stale         bool
	lastSync      time.Time
	loading       bool
	pointerInside bool
	noAutoDismiss bool

	dismissTimer uint
	onJoin       func(url string)

	// Stable callback references to avoid exhausting purego callback slots.
	eventRowClickCb stableCallback[func(gtk.GestureClick, int, float64, float64)]
	joinClickCb     stableCallback[func(gtk.Button)]
	backBtnClickCb  stableCallback[func(gtk.Button)]
	updateListCb    stableCallback[glib.SourceFunc]
	updateStatusCb  stableCallback[glib.SourceFunc]
	showCb          stableCallback[glib.SourceFunc]
	hideCb          stableCallback[glib.SourceFunc]
	toggleCb        stableCallback[glib.SourceFunc]
	dismissTimerCb  stableCallback[glib.SourceFunc]

	// Widget -> data lookup for stable callbacks (accessed only from GTK main thread)
	widgetEvents map[uintptr]*calendar.Event
	widgetLinks  map[uintptr]string
}

// Stable callback getters - these return pointers to the same function each time,
// allowing puregotk to reuse the same purego callback slot.

func (p *Popup) getEventRowClickCb() *func(gtk.GestureClick, int, float64, float64) {
	return p.eventRowClickCb.get(func() func(gtk.GestureClick, int, float64, float64) {
		return func(gesture gtk.GestureClick, nPress int, x, y float64) {
			widget := gesture.GetWidget()
			if widget == nil {
				return
			}
			if event, ok := p.widgetEvents[widget.GoPointer()]; ok {
				p.showDetails(*event)
			}
		}
	})
}

func (p *Popup) getJoinClickCb() *func(gtk.Button) {
	return p.joinClickCb.get(func() func(gtk.Button) {
		return func(btn gtk.Button) {
			if link, ok := p.widgetLinks[btn.GoPointer()]; ok {
				slog.Debug("join clicked", "url", link)
				if p.onJoin != nil {
					p.onJoin(link)
				} else {
					links.Open(link)
				}
				p.Hide()
			}
		}
	})
}

func (p *Popup) getBackBtnClickCb() *func(gtk.Button) {
	return p.backBtnClickCb.get(func() func(gtk.Button) {
		return func(btn gtk.Button) {
			p.hideDetails()
		}
	})
}

func (p *Popup) getUpdateListCb() *glib.SourceFunc {
	return p.updateListCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			p.updateList()
			return false
		}
	})
}

func (p *Popup) getUpdateStatusCb() *glib.SourceFunc {
	return p.updateStatusCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			p.updateStatusBar()
			return false
		}
	})
}

func (p *Popup) getShowCb() *glib.SourceFunc {
	return p.showCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			// Reset to list view when showing
			if p.stack != nil {
				p.stack.SetVisibleChildName("list")
			}
			p.detailsEvent = nil
			p.updateList()
			p.window.SetVisible(true)
			p.window.Present()
			return false
		}
	})
}

func (p *Popup) getHideCb() *glib.SourceFunc {
	return p.hideCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			p.hideAll()
			return false
		}
	})
}

func (p *Popup) getToggleCb() *glib.SourceFunc {
	return p.toggleCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			if p.window.IsVisible() {
				p.hideAll()
			} else {
				// Reset to list view when showing
				if p.stack != nil {
					p.stack.SetVisibleChildName("list")
				}
				p.detailsEvent = nil
				p.updateList()
				p.window.SetVisible(true)
				p.window.Present()
			}
			return false
		}
	})
}

func (p *Popup) getDismissTimerCb() *glib.SourceFunc {
	return p.dismissTimerCb.get(func() glib.SourceFunc {
		return func(data uintptr) bool {
			p.mu.RLock()
			pointerInside := p.pointerInside
			p.mu.RUnlock()
			// Double-check: only dismiss if still not active and pointer still outside
			if p.window.IsVisible() && !p.window.IsActive() && !pointerInside {
				p.hideAll()
			}
			p.dismissTimer = 0
			return false
		}
	})
}

// NewPopup creates a new popup window.
func NewPopup(timeRange, eventEndGrace time.Duration, noAutoDismiss bool) *Popup {
	return &Popup{
		timeRange:     timeRange,
		eventEndGrace: eventEndGrace,
		loading:       true,
		noAutoDismiss: noAutoDismiss,
	}
}

// Init initializes the GTK widgets. Must be called from GTK main thread.
func (p *Popup) Init() {
	// Initialize widget -> data lookup maps
	p.widgetEvents = make(map[uintptr]*calendar.Event)
	p.widgetLinks = make(map[uintptr]string)

	// Initialize libadwaita for automatic dark/light mode support
	adw.Init()

	p.window = gtk.NewWindow()
	p.window.SetTitle("CalBar")
	p.window.SetDefaultSize(380, 580)

	// Layer shell setup for Wayland compositors
	if gtk4layershell.IsSupported() {
		slog.Debug("layer shell supported")
		winPtr := p.window.GoPointer()
		gtk4layershell.InitForWindow(winPtr)
		gtk4layershell.SetLayer(winPtr, gtk4layershell.LayerTop)
		gtk4layershell.SetAnchor(winPtr, gtk4layershell.EdgeTop, true)
		gtk4layershell.SetAnchor(winPtr, gtk4layershell.EdgeRight, true)
		gtk4layershell.SetMargin(winPtr, gtk4layershell.EdgeTop, 8)
		gtk4layershell.SetMargin(winPtr, gtk4layershell.EdgeRight, 8)
		gtk4layershell.SetKeyboardMode(winPtr, gtk4layershell.KeyboardModeOnDemand)
		gtk4layershell.SetNamespace(winPtr, "calbar-popup")
		p.window.SetDecorated(false)

		// Auto-dismiss on focus loss (unless disabled)
		if !p.noAutoDismiss {
			// Connect to notify::is-active signal
			notifyCb := func(obj gobject.Object, pspec uintptr) {
				if p.window.IsVisible() {
					if p.window.IsActive() {
						if p.dismissTimer != 0 {
							glib.SourceRemove(p.dismissTimer)
							p.dismissTimer = 0
						}
					} else {
						p.mu.RLock()
						loading := p.loading
						pointerInside := p.pointerInside
						p.mu.RUnlock()
						// Only dismiss if pointer is also outside
						if !loading && !pointerInside && p.dismissTimer == 0 {
							p.startDismissTimer()
						}
					}
				}
			}
			p.window.ConnectNotify(&notifyCb)

			// Track pointer enter/leave for smart dismiss behavior
			motionController := gtk.NewEventControllerMotion()
			enterCb := func(ctrl gtk.EventControllerMotion, x, y float64) {
				slog.Debug("pointer entered popup", "x", x, "y", y)
				p.mu.Lock()
				p.pointerInside = true
				p.mu.Unlock()
				// Cancel any pending dismiss
				if p.dismissTimer != 0 {
					glib.SourceRemove(p.dismissTimer)
					p.dismissTimer = 0
				}
			}
			leaveCb := func(ctrl gtk.EventControllerMotion) {
				slog.Debug("pointer left popup")
				p.mu.Lock()
				p.pointerInside = false
				p.mu.Unlock()
				// If window not active and pointer left, start dismiss timer
				if p.window.IsVisible() && !p.window.IsActive() {
					p.startDismissTimer()
				}
			}
			motionController.ConnectEnter(&enterCb)
			motionController.ConnectLeave(&leaveCb)
			p.window.AddController(&motionController.EventController)
		}
	}

	// Hide on close request
	closeRequestCb := func(w gtk.Window) bool {
		p.window.SetVisible(false)
		return true
	}
	p.window.ConnectCloseRequest(&closeRequestCb)

	// Escape to close (or go back from details)
	keyController := gtk.NewEventControllerKey()
	keyPressedCb := func(ctrl gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == uint(gdk.KEY_Escape) {
			// If showing details, go back to list
			if p.stack != nil && p.stack.GetVisibleChildName() == "details" {
				p.hideDetails()
				return true
			}
			p.hideAll()
			return true
		}
		return false
	}
	keyController.ConnectKeyPressed(&keyPressedCb)
	p.window.AddController(&keyController.EventController)

	// Build UI
	p.buildUI()
	p.applyCSS()
	p.updateList()
}

// buildUI constructs the widget hierarchy.
func (p *Popup) buildUI() {
	// Main container
	p.content = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	p.content.AddCssClass("popup-container")
	p.window.SetChild(&p.content.Widget)

	// Stack for switching between list and details views
	p.stack = gtk.NewStack()
	p.stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRightValue)
	p.stack.SetTransitionDuration(200)
	p.stack.SetVexpand(true)
	p.content.Append(&p.stack.Widget)

	// List view (contains header, scroll, and all-day section)
	p.listView = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	p.stack.AddNamed(&p.listView.Widget, "list")

	// Header
	header := p.buildHeader()
	p.listView.Append(&header.Widget)

	// Scrolled event list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVexpand(true)
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.AddCssClass("event-scroll")
	p.listView.Append(&scrolled.Widget)

	p.listBox = gtk.NewListBox()
	p.listBox.SetSelectionMode(gtk.SelectionNoneValue)
	p.listBox.AddCssClass("event-list")
	scrolled.SetChild(&p.listBox.Widget)

	// All-day section (fixed at bottom, outside scroll)
	p.allDaySection = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	p.allDaySection.AddCssClass("all-day-section")
	p.allDaySection.SetVisible(false) // Hidden until populated
	p.listView.Append(&p.allDaySection.Widget)

	// Details view
	p.detailsView = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	p.stack.AddNamed(&p.detailsView.Widget, "details")

	// Status bar (always visible at bottom, outside stack)
	p.statusBar = gtk.NewLabel("")
	p.statusBar.AddCssClass("status-bar")
	p.statusBar.SetXalign(0)
	p.content.Append(&p.statusBar.Widget)
}

// buildHeader creates the header section.
func (p *Popup) buildHeader() *gtk.Box {
	header := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	header.AddCssClass("popup-header")

	// Calendar icon
	icon := gtk.NewImageFromIconName("x-office-calendar-symbolic")
	icon.AddCssClass("header-icon")
	icon.SetPixelSize(20)
	header.Append(&icon.Widget)

	// Title
	title := gtk.NewLabel("Upcoming Events")
	title.AddCssClass("header-title")
	title.SetHexpand(true)
	title.SetXalign(0)
	header.Append(&title.Widget)

	return header
}

// applyCSS applies custom styling with libadwaita color variables.
func (p *Popup) applyCSS() {
	css := `
		/* Main container */
		.popup-container {
			background: @window_bg_color;
			border-radius: 12px;
			border: 1px solid alpha(@borders, 0.5);
		}

		/* Header */
		.popup-header {
			padding: 16px 16px 12px 16px;
			border-bottom: 1px solid alpha(@borders, 0.3);
		}

		.header-icon {
			margin-right: 10px;
			color: @accent_color;
		}

		.header-title {
			font-size: 15px;
			font-weight: 600;
			letter-spacing: 0.3px;
		}

		/* Event list */
		.event-scroll {
			background: transparent;
		}

		.event-list {
			background: transparent;
		}

		.event-list > row {
			padding: 0;
			background: transparent;
			border: none;
		}

		.event-list > row:hover {
			background: alpha(@accent_color, 0.08);
		}

		/* Event card */
		.event-card {
			padding: 12px 16px;
			border-bottom: 1px solid alpha(@borders, 0.2);
		}

		/* Time indicator on the left */
		.time-indicator {
			min-width: 52px;
			margin-right: 12px;
		}

		.time-primary {
			font-size: 14px;
			font-weight: 600;
			color: @view_fg_color;
		}

		.time-secondary {
			font-size: 11px;
			color: alpha(@view_fg_color, 0.6);
		}

		.time-indicator.imminent .time-primary {
			color: @warning_color;
		}

		.time-indicator.now .time-primary {
			color: @accent_color;
		}

		/* Event details */
		.event-details {
			min-width: 0;
		}

		.event-title {
			font-size: 14px;
			font-weight: 500;
			color: @view_fg_color;
		}

		.event-title.ongoing {
			color: @accent_color;
		}

		.event-meta {
			font-size: 12px;
			color: alpha(@view_fg_color, 0.6);
			margin-top: 2px;
		}

		.event-source {
			font-size: 11px;
			color: alpha(@view_fg_color, 0.5);
			margin-top: 4px;
		}

		/* Join button */
		.join-btn {
			min-height: 28px;
			min-width: 28px;
			padding: 0 12px;
			border-radius: 8px;
			font-size: 12px;
			font-weight: 500;
			margin-left: 8px;
			background: @accent_bg_color;
			color: @accent_fg_color;
		}

		.join-btn:hover {
			filter: brightness(1.1);
		}

		/* Status bar */
		.status-bar {
			padding: 8px 16px;
			font-size: 11px;
			color: alpha(@view_fg_color, 0.5);
			border-top: 1px solid alpha(@borders, 0.2);
			background: alpha(@view_bg_color, 0.5);
			border-radius: 0 0 12px 12px;
		}

		.status-bar.stale {
			color: @warning_color;
		}

		/* Empty state */
		.empty-state {
			padding: 48px 24px;
		}

		.empty-icon {
			opacity: 0.3;
			margin-bottom: 16px;
		}

		.empty-title {
			font-size: 16px;
			font-weight: 600;
			color: alpha(@view_fg_color, 0.6);
			margin-bottom: 4px;
		}

		.empty-subtitle {
			font-size: 13px;
			color: alpha(@view_fg_color, 0.4);
		}

		/* Loading state */
		.loading-state {
			padding: 48px 24px;
		}

		.loading-text {
			font-size: 13px;
			color: alpha(@view_fg_color, 0.6);
			margin-top: 12px;
		}

		/* Day separator */
		.day-separator {
			padding: 8px 16px 6px 16px;
			font-size: 11px;
			font-weight: 600;
			color: alpha(@view_fg_color, 0.5);
			text-transform: uppercase;
			letter-spacing: 0.5px;
			background: alpha(@view_bg_color, 0.3);
			border-bottom: 1px solid alpha(@borders, 0.2);
		}

		/* All-day section */
		.all-day-section {
			background: alpha(@view_bg_color, 0.3);
			border-top: 1px solid alpha(@borders, 0.3);
		}

		.all-day-header {
			padding: 8px 16px 6px 16px;
			font-size: 11px;
			font-weight: 600;
			color: alpha(@view_fg_color, 0.4);
			text-transform: uppercase;
			letter-spacing: 0.5px;
		}

		.all-day-row {
			padding: 8px 16px;
			border-bottom: 1px solid alpha(@borders, 0.15);
		}

		.all-day-row:last-child {
			border-bottom: none;
		}

		.all-day-title {
			font-size: 13px;
			font-weight: 400;
			color: alpha(@view_fg_color, 0.6);
		}

		.all-day-meta {
			font-size: 11px;
			color: alpha(@view_fg_color, 0.4);
			margin-top: 2px;
		}

		/* Details panel */
		.details-header {
			padding: 12px 16px;
			border-bottom: 1px solid alpha(@borders, 0.3);
		}

		.details-back-btn {
			min-width: 32px;
			min-height: 32px;
			padding: 0;
			border-radius: 8px;
			background: transparent;
		}

		.details-back-btn:hover {
			background: alpha(@view_fg_color, 0.1);
		}

		.details-content {
			padding: 16px;
		}

		.details-title {
			font-size: 18px;
			font-weight: 600;
			color: @view_fg_color;
			margin-bottom: 16px;
		}

		.details-row {
			margin-bottom: 8px;
		}

		.details-icon {
			min-width: 24px;
			font-size: 14px;
		}

		.details-text {
			font-size: 14px;
			color: @view_fg_color;
		}

		.details-description-section {
			margin-top: 16px;
			padding-top: 16px;
			border-top: 1px solid alpha(@borders, 0.2);
		}

		.details-section-label {
			font-size: 12px;
			font-weight: 600;
			color: alpha(@view_fg_color, 0.5);
			text-transform: uppercase;
			letter-spacing: 0.5px;
			margin-bottom: 8px;
		}

		.details-description {
			font-size: 13px;
			color: alpha(@view_fg_color, 0.8);
			line-height: 1.5;
		}

		.details-join-box {
			margin-top: 24px;
			padding-top: 16px;
			border-top: 1px solid alpha(@borders, 0.2);
		}

		.details-join-btn {
			min-width: 120px;
		}

		/* Make event rows look clickable */
		.event-card {
			cursor: pointer;
		}

		.all-day-row {
			cursor: pointer;
		}

		.all-day-row:hover {
			background: alpha(@accent_color, 0.08);
		}
	`

	provider := gtk.NewCssProvider()
	provider.LoadFromString(css)

	if display := gdk.DisplayGetDefault(); display != nil {
		gtk.StyleContextAddProviderForDisplay(display, provider, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
	}
}

// Show shows the popup window.
func (p *Popup) Show() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(p.getShowCb(), 0)
}

// Hide hides the popup window.
func (p *Popup) Hide() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(p.getHideCb(), 0)
}

func (p *Popup) hideAll() {
	p.window.SetVisible(false)
	if p.dismissTimer != 0 {
		glib.SourceRemove(p.dismissTimer)
		p.dismissTimer = 0
	}
}

// startDismissTimer starts a timer to dismiss the popup after a short delay.
func (p *Popup) startDismissTimer() {
	if p.dismissTimer != 0 {
		return
	}
	p.dismissTimer = glib.TimeoutAdd(300, p.getDismissTimerCb(), 0)
}

// Toggle shows or hides the popup.
func (p *Popup) Toggle() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(p.getToggleCb(), 0)
}

// SetEvents updates the event list.
func (p *Popup) SetEvents(events []calendar.Event) {
	p.mu.Lock()
	p.events = events
	p.lastSync = time.Now()
	p.stale = false
	p.loading = false
	p.mu.Unlock()

	glib.IdleAdd(p.getUpdateListCb(), 0)
}

// SetStale marks the data as potentially stale.
func (p *Popup) SetStale(stale bool) {
	p.mu.Lock()
	p.stale = stale
	p.mu.Unlock()

	glib.IdleAdd(p.getUpdateStatusCb(), 0)
}

// OnJoin sets the callback for when a join button is clicked.
func (p *Popup) OnJoin(fn func(url string)) {
	p.onJoin = fn
}

// updateList refreshes the event list UI.
func (p *Popup) updateList() {
	if p.listBox == nil {
		return
	}

	// Clear widget -> data lookup maps before rebuilding
	p.widgetEvents = make(map[uintptr]*calendar.Event)
	p.widgetLinks = make(map[uintptr]string)

	// Clear existing timed events
	for child := p.listBox.GetFirstChild(); child != nil; child = p.listBox.GetFirstChild() {
		p.listBox.Remove(child)
	}

	// Clear existing all-day events
	for child := p.allDaySection.GetFirstChild(); child != nil; child = p.allDaySection.GetFirstChild() {
		p.allDaySection.Remove(child)
	}
	p.allDaySection.SetVisible(false)

	p.mu.RLock()
	events := p.events
	timeRange := p.timeRange
	eventEndGrace := p.eventEndGrace
	loading := p.loading
	p.mu.RUnlock()

	if loading {
		p.showLoadingState()
		p.updateStatusBar()
		return
	}

	now := time.Now()
	cutoff := now.Add(timeRange)
	// Get today in local time for all-day event filtering
	localNow := now.Local()
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)

	// Separate timed events and all-day events
	var timedEvents []calendar.Event
	var allDayEvents []calendar.Event

	for _, e := range events {
		// Keep events visible for a grace period after they end
		if e.End.Add(eventEndGrace).Before(now) {
			continue
		}
		if e.Start.After(cutoff) {
			continue
		}

		if e.AllDay {
			// Only include all-day events that span today (compare in local time)
			localStart := e.Start.Local()
			localEnd := e.End.Local()
			eventStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, time.Local)
			eventEnd := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, time.Local)
			if !today.Before(eventStart) && today.Before(eventEnd) {
				allDayEvents = append(allDayEvents, e)
			}
		} else {
			timedEvents = append(timedEvents, e)
		}
	}

	// Sort timed events by start time
	sort.Slice(timedEvents, func(i, j int) bool {
		return timedEvents[i].Start.Before(timedEvents[j].Start)
	})

	// Sort all-day events alphabetically
	sort.Slice(allDayEvents, func(i, j int) bool {
		return allDayEvents[i].Summary < allDayEvents[j].Summary
	})

	if len(timedEvents) == 0 && len(allDayEvents) == 0 {
		p.showEmptyState()
	} else {
		if len(timedEvents) > 0 {
			p.populateTimedEvents(timedEvents, now)
		} else {
			// Show minimal empty state for timed events when only all-day exist
			p.showNoTimedEventsState()
		}
		if len(allDayEvents) > 0 {
			p.populateAllDayEvents(allDayEvents, now)
		}
	}

	p.updateStatusBar()
}

// showLoadingState displays the loading indicator.
func (p *Popup) showLoadingState() {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("loading-state")
	box.SetHalign(gtk.AlignCenterValue)
	box.SetValign(gtk.AlignCenterValue)
	box.SetVexpand(true)

	spinner := gtk.NewSpinner()
	spinner.SetSizeRequest(32, 32)
	spinner.Start()
	box.Append(&spinner.Widget)

	label := gtk.NewLabel("Loading calendars...")
	label.AddCssClass("loading-text")
	box.Append(&label.Widget)

	p.listBox.Append(&box.Widget)
}

// showEmptyState displays the empty state.
func (p *Popup) showEmptyState() {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("empty-state")
	box.SetHalign(gtk.AlignCenterValue)
	box.SetValign(gtk.AlignCenterValue)
	box.SetVexpand(true)

	icon := gtk.NewImageFromIconName("weather-clear-symbolic")
	icon.AddCssClass("empty-icon")
	icon.SetPixelSize(48)
	box.Append(&icon.Widget)

	title := gtk.NewLabel("All Clear")
	title.AddCssClass("empty-title")
	box.Append(&title.Widget)

	subtitle := gtk.NewLabel("No upcoming events")
	subtitle.AddCssClass("empty-subtitle")
	box.Append(&subtitle.Widget)

	p.listBox.Append(&box.Widget)
}

// showNoTimedEventsState displays a minimal state when only all-day events exist.
func (p *Popup) showNoTimedEventsState() {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("empty-state")
	box.SetHalign(gtk.AlignCenterValue)
	box.SetValign(gtk.AlignCenterValue)
	box.SetVexpand(true)

	icon := gtk.NewImageFromIconName("weather-clear-symbolic")
	icon.AddCssClass("empty-icon")
	icon.SetPixelSize(32)
	box.Append(&icon.Widget)

	subtitle := gtk.NewLabel("No timed events today")
	subtitle.AddCssClass("empty-subtitle")
	box.Append(&subtitle.Widget)

	p.listBox.Append(&box.Widget)
}

// populateTimedEvents adds timed event rows grouped by day.
func (p *Popup) populateTimedEvents(events []calendar.Event, now time.Time) {
	var lastDay string

	for _, event := range events {
		// Day separator
		day := p.getDayLabel(event.Start, now)
		if day != lastDay {
			sep := gtk.NewLabel(day)
			sep.AddCssClass("day-separator")
			sep.SetXalign(0)
			p.listBox.Append(&sep.Widget)
			lastDay = day
		}

		row := p.createTimedEventRow(event, now)
		p.listBox.Append(&row.Widget)
	}
}

// populateAllDayEvents adds the all-day events to the fixed bottom section.
func (p *Popup) populateAllDayEvents(events []calendar.Event, now time.Time) {
	// Header
	header := gtk.NewLabel("All Day")
	header.AddCssClass("all-day-header")
	header.SetXalign(0)
	p.allDaySection.Append(&header.Widget)

	// Event rows
	for _, event := range events {
		row := p.createAllDayEventRow(event, now)
		p.allDaySection.Append(&row.Widget)
	}

	p.allDaySection.SetVisible(true)
}

// getDayLabel returns a human-readable day label.
func (p *Popup) getDayLabel(t time.Time, now time.Time) string {
	// Convert to local time for day comparison
	localTime := t.Local()
	localNow := now.Local()

	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	eventDay := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, time.Local)

	switch {
	case eventDay.Equal(today):
		return "Today"
	case eventDay.Equal(today.Add(24 * time.Hour)):
		return "Tomorrow"
	default:
		return localTime.Format("Monday, Jan 2")
	}
}

// createTimedEventRow creates a styled row for a timed event.
func (p *Popup) createTimedEventRow(event calendar.Event, now time.Time) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	row.AddCssClass("event-card")

	// Make row clickable to show details
	clickGesture := gtk.NewGestureClick()
	eventCopy := event // Store for lookup
	p.widgetEvents[row.GoPointer()] = &eventCopy
	clickGesture.ConnectReleased(p.getEventRowClickCb())
	row.AddController(&clickGesture.EventController)

	// Time indicator
	timeBox := p.createTimeIndicator(event, now)
	row.Append(&timeBox.Widget)

	// Event details
	details := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	details.AddCssClass("event-details")
	details.SetHexpand(true)
	row.Append(&details.Widget)

	// Title
	title := gtk.NewLabel(event.Summary)
	title.AddCssClass("event-title")
	title.SetXalign(0)
	title.SetEllipsize(pango.EllipsizeEndValue)
	title.SetMaxWidthChars(35)
	if event.IsOngoing(now) {
		title.AddCssClass("ongoing")
	}
	details.Append(&title.Widget)

	// Meta info (duration)
	meta := p.getEventDuration(event)
	if meta != "" {
		metaLabel := gtk.NewLabel(meta)
		metaLabel.AddCssClass("event-meta")
		metaLabel.SetXalign(0)
		metaLabel.SetEllipsize(pango.EllipsizeEndValue)
		details.Append(&metaLabel.Widget)
	}

	// Source
	if event.Source != "" {
		source := gtk.NewLabel(event.Source)
		source.AddCssClass("event-source")
		source.SetXalign(0)
		details.Append(&source.Widget)
	}

	// Join button
	if meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL); meetingLink != "" {
		btn := p.createJoinButton(meetingLink)
		row.Append(&btn.Widget)
	}

	return row
}

// createAllDayEventRow creates a compact row for an all-day event.
func (p *Popup) createAllDayEventRow(event calendar.Event, now time.Time) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	row.AddCssClass("all-day-row")

	// Make row clickable to show details
	clickGesture := gtk.NewGestureClick()
	eventCopy := event // Store for lookup
	p.widgetEvents[row.GoPointer()] = &eventCopy
	clickGesture.ConnectReleased(p.getEventRowClickCb())
	row.AddController(&clickGesture.EventController)

	// Title
	title := gtk.NewLabel(event.Summary)
	title.AddCssClass("all-day-title")
	title.SetXalign(0)
	title.SetEllipsize(pango.EllipsizeEndValue)
	row.Append(&title.Widget)

	// Meta: date range + source
	var metaParts []string
	dateRange := p.formatDateRange(event, now)
	if dateRange != "" {
		metaParts = append(metaParts, dateRange)
	}
	if event.Source != "" {
		metaParts = append(metaParts, event.Source)
	}

	if len(metaParts) > 0 {
		metaText := ""
		for i, part := range metaParts {
			if i > 0 {
				metaText += " • "
			}
			metaText += part
		}
		meta := gtk.NewLabel(metaText)
		meta.AddCssClass("all-day-meta")
		meta.SetXalign(0)
		row.Append(&meta.Widget)
	}

	return row
}

// createTimeIndicator creates the time display on the left.
func (p *Popup) createTimeIndicator(event calendar.Event, now time.Time) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.AddCssClass("time-indicator")
	box.SetValign(gtk.AlignCenterValue)

	var primary, secondary string

	// Convert to local time for display
	localStart := event.Start.Local()

	if event.IsOngoing(now) {
		box.AddCssClass("now")
		primary = "Now"
		remaining := event.End.Sub(now)
		if remaining < time.Hour {
			secondary = fmt.Sprintf("%dm left", int(remaining.Minutes()))
		} else {
			secondary = fmt.Sprintf("%.1fh left", remaining.Hours())
		}
	} else {
		startsIn := event.Start.Sub(now)
		primary = localStart.Format("3:04")
		secondary = localStart.Format("PM")

		if startsIn <= 15*time.Minute && startsIn > 0 {
			box.AddCssClass("imminent")
			primary = fmt.Sprintf("%dm", int(startsIn.Minutes()))
			secondary = "away"
		}
	}

	primaryLabel := gtk.NewLabel(primary)
	primaryLabel.AddCssClass("time-primary")
	box.Append(&primaryLabel.Widget)

	secondaryLabel := gtk.NewLabel(secondary)
	secondaryLabel.AddCssClass("time-secondary")
	box.Append(&secondaryLabel.Widget)

	return box
}

// getEventDuration returns the event duration as a string.
func (p *Popup) getEventDuration(event calendar.Event) string {
	duration := event.End.Sub(event.Start)
	if duration < time.Hour {
		return fmt.Sprintf("%d min", int(duration.Minutes()))
	}
	if duration == time.Hour {
		return "1 hour"
	}
	hours := duration.Hours()
	if hours == float64(int(hours)) {
		return fmt.Sprintf("%d hours", int(hours))
	}
	return fmt.Sprintf("%.1f hours", hours)
}

// formatDateRange returns the date range for an all-day event.
func (p *Popup) formatDateRange(event calendar.Event, now time.Time) string {
	// Convert to local for display
	localStart := event.Start.Local()
	localEnd := event.End.Local()
	localNow := now.Local()

	startDay := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, time.Local)
	// All-day events have exclusive end dates, so subtract a day
	endDay := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, time.Local).Add(-24 * time.Hour)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)

	// Single day event
	if startDay.Equal(endDay) {
		if startDay.Equal(today) {
			return "Today"
		}
		return startDay.Format("Jan 2")
	}

	// Multi-day event
	startStr := startDay.Format("Jan 2")
	endStr := endDay.Format("Jan 2")

	// Use relative names where possible
	if startDay.Equal(today) {
		startStr = "Today"
	} else if startDay.Equal(today.Add(-24 * time.Hour)) {
		startStr = "Yesterday"
	}

	if endDay.Equal(today) {
		endStr = "Today"
	} else if endDay.Equal(today.Add(24 * time.Hour)) {
		endStr = "Tomorrow"
	}

	return fmt.Sprintf("%s – %s", startStr, endStr)
}

// createJoinButton creates a meeting join button.
func (p *Popup) createJoinButton(meetingLink string) *gtk.Button {
	service := links.Service(meetingLink)

	btn := gtk.NewButton()
	btn.AddCssClass("join-btn")

	// Use icon + label for known services
	box := gtk.NewBox(gtk.OrientationHorizontalValue, 4)

	var iconName string
	switch service {
	case "Teams":
		iconName = "video-display-symbolic"
	case "Zoom":
		iconName = "camera-video-symbolic"
	case "Meet":
		iconName = "camera-web-symbolic"
	default:
		iconName = "video-joined-displays-symbolic"
	}

	icon := gtk.NewImageFromIconName(iconName)
	icon.SetPixelSize(14)
	box.Append(&icon.Widget)

	label := gtk.NewLabel("Join")
	box.Append(&label.Widget)

	btn.SetChild(&box.Widget)

	// Store link for lookup and use stable callback
	p.widgetLinks[btn.GoPointer()] = meetingLink
	btn.ConnectClicked(p.getJoinClickCb())

	return btn
}

// updateStatusBar updates the status bar text.
func (p *Popup) updateStatusBar() {
	if p.statusBar == nil {
		return
	}

	p.mu.RLock()
	stale := p.stale
	lastSync := p.lastSync
	eventCount := len(p.events)
	loading := p.loading
	p.mu.RUnlock()

	p.statusBar.RemoveCssClass("stale")

	var text string
	switch {
	case loading:
		text = "Syncing..."
	case stale:
		text = fmt.Sprintf("⚠ Data may be stale • Last sync: %s", lastSync.Format("3:04 PM"))
		p.statusBar.AddCssClass("stale")
	case lastSync.IsZero():
		text = "Waiting for sync..."
	default:
		text = fmt.Sprintf("%d events • Synced %s", eventCount, lastSync.Format("3:04 PM"))
	}

	p.statusBar.SetText(text)
}

// showDetails displays the event details panel.
func (p *Popup) showDetails(event calendar.Event) {
	p.detailsEvent = &event

	// Clear previous details content
	for child := p.detailsView.GetFirstChild(); child != nil; child = p.detailsView.GetFirstChild() {
		p.detailsView.Remove(child)
	}

	// Build details header with back button
	detailsHeader := gtk.NewBox(gtk.OrientationHorizontalValue, 8)
	detailsHeader.AddCssClass("details-header")

	backBtn := gtk.NewButton()
	backBtn.SetIconName("go-previous-symbolic")
	backBtn.AddCssClass("details-back-btn")
	backBtn.ConnectClicked(p.getBackBtnClickCb())
	detailsHeader.Append(&backBtn.Widget)

	headerTitle := gtk.NewLabel("Event Details")
	headerTitle.AddCssClass("header-title")
	headerTitle.SetHexpand(true)
	headerTitle.SetXalign(0)
	detailsHeader.Append(&headerTitle.Widget)

	p.detailsView.Append(&detailsHeader.Widget)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVexpand(true)
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	p.detailsView.Append(&scrolled.Widget)

	content := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	content.AddCssClass("details-content")
	scrolled.SetChild(&content.Widget)

	// Event title
	title := gtk.NewLabel(event.Summary)
	title.AddCssClass("details-title")
	title.SetXalign(0)
	title.SetWrap(true)
	title.SetWrapMode(pango.WrapWordCharValue)
	content.Append(&title.Widget)

	// Time and date
	now := time.Now()
	localStart := event.Start.Local()
	localEnd := event.End.Local()

	var timeStr string
	if event.AllDay {
		timeStr = p.formatDateRange(event, now)
	} else {
		dayLabel := p.getDayLabel(event.Start, now)
		timeStr = fmt.Sprintf("%s • %s – %s", dayLabel, localStart.Format("3:04 PM"), localEnd.Format("3:04 PM"))
	}
	p.addDetailRow(content, "📅", timeStr)

	// Duration (for non-all-day events)
	if !event.AllDay {
		duration := p.getEventDuration(event)
		p.addDetailRow(content, "⏱", duration)
	}

	// Location
	if event.Location != "" {
		p.addDetailRow(content, "📍", event.Location)
	}

	// Organizer
	if event.Organizer != "" {
		p.addDetailRow(content, "👤", event.Organizer)
	}

	// Source
	if event.Source != "" {
		p.addDetailRow(content, "📁", event.Source)
	}

	// Description
	if event.Description != "" {
		descSection := gtk.NewBox(gtk.OrientationVerticalValue, 4)
		descSection.AddCssClass("details-description-section")

		descLabel := gtk.NewLabel("Description")
		descLabel.AddCssClass("details-section-label")
		descLabel.SetXalign(0)
		descSection.Append(&descLabel.Widget)

		// Strip HTML and clean up description
		cleanDesc := stripHTML(event.Description)
		descText := gtk.NewLabel(cleanDesc)
		descText.AddCssClass("details-description")
		descText.SetXalign(0)
		descText.SetWrap(true)
		descText.SetWrapMode(pango.WrapWordCharValue)
		descText.SetSelectable(true)
		descSection.Append(&descText.Widget)

		content.Append(&descSection.Widget)
	}

	// Join button (if meeting link exists)
	if meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL); meetingLink != "" {
		btnBox := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
		btnBox.AddCssClass("details-join-box")
		btnBox.SetHalign(gtk.AlignCenterValue)

		joinBtn := p.createJoinButton(meetingLink)
		joinBtn.AddCssClass("details-join-btn")
		btnBox.Append(&joinBtn.Widget)

		content.Append(&btnBox.Widget)
	}

	// Switch to details view
	p.stack.SetVisibleChildName("details")
}

// hideDetails returns to the event list view.
func (p *Popup) hideDetails() {
	p.stack.SetVisibleChildName("list")
	p.detailsEvent = nil
}

// addDetailRow adds a row with icon and text to the details panel.
func (p *Popup) addDetailRow(container *gtk.Box, icon string, text string) {
	row := gtk.NewBox(gtk.OrientationHorizontalValue, 8)
	row.AddCssClass("details-row")

	iconLabel := gtk.NewLabel(icon)
	iconLabel.AddCssClass("details-icon")
	row.Append(&iconLabel.Widget)

	textLabel := gtk.NewLabel(text)
	textLabel.AddCssClass("details-text")
	textLabel.SetXalign(0)
	textLabel.SetWrap(true)
	textLabel.SetWrapMode(pango.WrapWordCharValue)
	textLabel.SetSelectable(true)
	row.Append(&textLabel.Widget)

	container.Append(&row.Widget)
}

// stripHTML removes HTML tags and converts to readable plain text.
func stripHTML(s string) string {
	var result []byte
	inTag := false
	tagName := ""
	lastWasNewline := false
	lastWasSpace := false

	// Helper to add newline(s)
	addNewline := func(count int) {
		// Don't add newlines at the start
		if len(result) == 0 {
			return
		}
		// Count existing trailing newlines
		existing := 0
		for i := len(result) - 1; i >= 0 && result[i] == '\n'; i-- {
			existing++
		}
		// Add only what's needed up to count
		for i := existing; i < count; i++ {
			result = append(result, '\n')
		}
		lastWasNewline = true
		lastWasSpace = true
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		if c == '<' {
			inTag = true
			tagName = ""
			continue
		}

		if inTag {
			if c == '>' {
				inTag = false
				// Process tag for formatting
				tagLower := strings.ToLower(tagName)
				// Remove leading slash for closing tags
				isClosing := strings.HasPrefix(tagLower, "/")
				if isClosing {
					tagLower = tagLower[1:]
				}
				// Extract just the tag name (before any attributes)
				if spaceIdx := strings.IndexAny(tagLower, " \t\n"); spaceIdx > 0 {
					tagLower = tagLower[:spaceIdx]
				}

				switch tagLower {
				case "br":
					addNewline(1)
				case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
					if isClosing {
						addNewline(2)
					}
				case "li":
					if !isClosing {
						addNewline(1)
						result = append(result, []byte("• ")...)
						lastWasSpace = true
					}
				case "tr":
					if isClosing {
						addNewline(1)
					}
				case "ul", "ol":
					if isClosing {
						addNewline(1)
					}
				}
				continue
			}
			tagName += string(c)
			continue
		}

		// Handle HTML entities
		if c == '&' {
			entity := ""
			for j := i + 1; j < len(s) && j < i+10; j++ {
				if s[j] == ';' {
					entity = s[i+1 : j]
					i = j
					break
				}
				if s[j] == ' ' || s[j] == '<' {
					break
				}
			}
			if entity != "" {
				var replacement byte
				switch entity {
				case "nbsp":
					replacement = ' '
				case "amp":
					replacement = '&'
				case "lt":
					replacement = '<'
				case "gt":
					replacement = '>'
				case "quot":
					replacement = '"'
				case "#39", "apos":
					replacement = '\''
				case "#8217": // right single quote
					replacement = '\''
				case "#8216": // left single quote
					replacement = '\''
				case "#8220", "#8221": // double quotes
					replacement = '"'
				case "#8211": // en-dash
					replacement = '-'
				case "#8212": // em-dash
					replacement = '-'
				case "#160": // non-breaking space
					replacement = ' '
				default:
					// Skip unknown entities
					continue
				}
				if replacement == ' ' {
					if !lastWasSpace {
						result = append(result, replacement)
						lastWasSpace = true
					}
				} else {
					result = append(result, replacement)
					lastWasSpace = false
					lastWasNewline = false
				}
				continue
			}
		}

		// Handle whitespace
		if c == '\n' || c == '\r' {
			if !lastWasNewline && !lastWasSpace {
				result = append(result, ' ')
				lastWasSpace = true
			}
			continue
		}
		if c == '\t' || c == ' ' {
			if !lastWasSpace {
				result = append(result, ' ')
				lastWasSpace = true
			}
			continue
		}

		// Regular character
		result = append(result, c)
		lastWasSpace = false
		lastWasNewline = false
	}

	// Trim leading/trailing whitespace
	return strings.TrimSpace(string(result))
}
