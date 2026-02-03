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
	"github.com/cpuguy83/calbar/internal/links"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

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
	stale         bool
	lastSync      time.Time
	loading       bool
	pointerInside bool
	noAutoDismiss bool

	dismissTimer glib.SourceHandle
	onJoin       func(url string)
}

// NewPopup creates a new popup window.
func NewPopup(timeRange time.Duration, noAutoDismiss bool) *Popup {
	return &Popup{
		timeRange:     timeRange,
		loading:       true,
		noAutoDismiss: noAutoDismiss,
	}
}

// Init initializes the GTK widgets. Must be called from GTK main thread.
func (p *Popup) Init() {
	// Initialize libadwaita for automatic dark/light mode support
	adw.Init()

	p.window = gtk.NewWindow()
	p.window.SetTitle("CalBar")
	p.window.SetDefaultSize(380, 580)

	// Layer shell setup for Wayland compositors
	if gtk4layershell.IsSupported() {
		slog.Debug("layer shell supported")
		gtk4layershell.InitForWindow(p.window)
		gtk4layershell.SetLayer(p.window, gtk4layershell.LayerShellLayerTop)
		gtk4layershell.SetAnchor(p.window, gtk4layershell.LayerShellEdgeTop, true)
		gtk4layershell.SetAnchor(p.window, gtk4layershell.LayerShellEdgeRight, true)
		gtk4layershell.SetMargin(p.window, gtk4layershell.LayerShellEdgeTop, 8)
		gtk4layershell.SetMargin(p.window, gtk4layershell.LayerShellEdgeRight, 8)
		gtk4layershell.SetKeyboardMode(p.window, gtk4layershell.LayerShellKeyboardModeOnDemand)
		gtk4layershell.SetNamespace(p.window, "calbar-popup")
		p.window.SetDecorated(false)

		// Auto-dismiss on focus loss (unless disabled)
		if !p.noAutoDismiss {
			p.window.NotifyProperty("is-active", func() {
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
			})

			// Track pointer enter/leave for smart dismiss behavior
			motionController := gtk.NewEventControllerMotion()
			motionController.ConnectEnter(func(x, y float64) {
				slog.Debug("pointer entered popup", "x", x, "y", y)
				p.mu.Lock()
				p.pointerInside = true
				p.mu.Unlock()
				// Cancel any pending dismiss
				if p.dismissTimer != 0 {
					glib.SourceRemove(p.dismissTimer)
					p.dismissTimer = 0
				}
			})
			motionController.ConnectLeave(func() {
				slog.Debug("pointer left popup")
				p.mu.Lock()
				p.pointerInside = false
				p.mu.Unlock()
				// If window not active and pointer left, start dismiss timer
				if p.window.IsVisible() && !p.window.IsActive() {
					p.startDismissTimer()
				}
			})
			p.window.AddController(motionController)
		}
	}

	// Hide on close request
	p.window.ConnectCloseRequest(func() bool {
		p.window.SetVisible(false)
		return true
	})

	// Escape to close (or go back from details)
	keyController := gtk.NewEventControllerKey()
	keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			// If showing details, go back to list
			if p.stack != nil && p.stack.VisibleChildName() == "details" {
				p.hideDetails()
				return true
			}
			p.hideAll()
			return true
		}
		return false
	})
	p.window.AddController(keyController)

	// Build UI
	p.buildUI()
	p.applyCSS()
	p.updateList()
}

// buildUI constructs the widget hierarchy.
func (p *Popup) buildUI() {
	// Main container
	p.content = gtk.NewBox(gtk.OrientationVertical, 0)
	p.content.AddCSSClass("popup-container")
	p.window.SetChild(p.content)

	// Stack for switching between list and details views
	p.stack = gtk.NewStack()
	p.stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)
	p.stack.SetTransitionDuration(200)
	p.stack.SetVExpand(true)
	p.content.Append(p.stack)

	// List view (contains header, scroll, and all-day section)
	p.listView = gtk.NewBox(gtk.OrientationVertical, 0)
	p.stack.AddNamed(p.listView, "list")

	// Header
	header := p.buildHeader()
	p.listView.Append(header)

	// Scrolled event list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.AddCSSClass("event-scroll")
	p.listView.Append(scrolled)

	p.listBox = gtk.NewListBox()
	p.listBox.SetSelectionMode(gtk.SelectionNone)
	p.listBox.AddCSSClass("event-list")
	scrolled.SetChild(p.listBox)

	// All-day section (fixed at bottom, outside scroll)
	p.allDaySection = gtk.NewBox(gtk.OrientationVertical, 0)
	p.allDaySection.AddCSSClass("all-day-section")
	p.allDaySection.SetVisible(false) // Hidden until populated
	p.listView.Append(p.allDaySection)

	// Details view
	p.detailsView = gtk.NewBox(gtk.OrientationVertical, 0)
	p.stack.AddNamed(p.detailsView, "details")

	// Status bar (always visible at bottom, outside stack)
	p.statusBar = gtk.NewLabel("")
	p.statusBar.AddCSSClass("status-bar")
	p.statusBar.SetXAlign(0)
	p.content.Append(p.statusBar)
}

// buildHeader creates the header section.
func (p *Popup) buildHeader() *gtk.Box {
	header := gtk.NewBox(gtk.OrientationHorizontal, 0)
	header.AddCSSClass("popup-header")

	// Calendar icon
	icon := gtk.NewImageFromIconName("x-office-calendar-symbolic")
	icon.AddCSSClass("header-icon")
	icon.SetPixelSize(20)
	header.Append(icon)

	// Title
	title := gtk.NewLabel("Upcoming Events")
	title.AddCSSClass("header-title")
	title.SetHExpand(true)
	title.SetXAlign(0)
	header.Append(title)

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

	provider := gtk.NewCSSProvider()
	provider.LoadFromData(css)

	if display := gdk.DisplayGetDefault(); display != nil {
		gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}

// Show shows the popup window.
func (p *Popup) Show() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(func() {
		// Reset to list view when showing
		if p.stack != nil {
			p.stack.SetVisibleChildName("list")
		}
		p.detailsEvent = nil
		p.updateList()
		p.window.SetVisible(true)
		p.window.Present()
	})
}

// Hide hides the popup window.
func (p *Popup) Hide() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(p.hideAll)
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
	p.dismissTimer = glib.TimeoutAdd(300, func() bool {
		p.mu.RLock()
		pointerInside := p.pointerInside
		p.mu.RUnlock()
		// Double-check: only dismiss if still not active and pointer still outside
		if p.window.IsVisible() && !p.window.IsActive() && !pointerInside {
			p.hideAll()
		}
		p.dismissTimer = 0
		return false
	})
}

// Toggle shows or hides the popup.
func (p *Popup) Toggle() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(func() {
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
	})
}

// SetEvents updates the event list.
func (p *Popup) SetEvents(events []calendar.Event) {
	p.mu.Lock()
	p.events = events
	p.lastSync = time.Now()
	p.stale = false
	p.loading = false
	p.mu.Unlock()

	glib.IdleAdd(p.updateList)
}

// SetStale marks the data as potentially stale.
func (p *Popup) SetStale(stale bool) {
	p.mu.Lock()
	p.stale = stale
	p.mu.Unlock()

	glib.IdleAdd(p.updateStatusBar)
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

	// Clear existing timed events
	for child := p.listBox.FirstChild(); child != nil; child = p.listBox.FirstChild() {
		p.listBox.Remove(child)
	}

	// Clear existing all-day events
	for child := p.allDaySection.FirstChild(); child != nil; child = p.allDaySection.FirstChild() {
		p.allDaySection.Remove(child)
	}
	p.allDaySection.SetVisible(false)

	p.mu.RLock()
	events := p.events
	timeRange := p.timeRange
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
		if e.End.Before(now) {
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
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("loading-state")
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)
	box.SetVExpand(true)

	spinner := gtk.NewSpinner()
	spinner.SetSizeRequest(32, 32)
	spinner.Start()
	box.Append(spinner)

	label := gtk.NewLabel("Loading calendars...")
	label.AddCSSClass("loading-text")
	box.Append(label)

	p.listBox.Append(box)
}

// showEmptyState displays the empty state.
func (p *Popup) showEmptyState() {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("empty-state")
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)
	box.SetVExpand(true)

	icon := gtk.NewImageFromIconName("weather-clear-symbolic")
	icon.AddCSSClass("empty-icon")
	icon.SetPixelSize(48)
	box.Append(icon)

	title := gtk.NewLabel("All Clear")
	title.AddCSSClass("empty-title")
	box.Append(title)

	subtitle := gtk.NewLabel("No upcoming events")
	subtitle.AddCSSClass("empty-subtitle")
	box.Append(subtitle)

	p.listBox.Append(box)
}

// showNoTimedEventsState displays a minimal state when only all-day events exist.
func (p *Popup) showNoTimedEventsState() {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("empty-state")
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)
	box.SetVExpand(true)

	icon := gtk.NewImageFromIconName("weather-clear-symbolic")
	icon.AddCSSClass("empty-icon")
	icon.SetPixelSize(32)
	box.Append(icon)

	subtitle := gtk.NewLabel("No timed events today")
	subtitle.AddCSSClass("empty-subtitle")
	box.Append(subtitle)

	p.listBox.Append(box)
}

// populateTimedEvents adds timed event rows grouped by day.
func (p *Popup) populateTimedEvents(events []calendar.Event, now time.Time) {
	var lastDay string

	for _, event := range events {
		// Day separator
		day := p.getDayLabel(event.Start, now)
		if day != lastDay {
			sep := gtk.NewLabel(day)
			sep.AddCSSClass("day-separator")
			sep.SetXAlign(0)
			p.listBox.Append(sep)
			lastDay = day
		}

		row := p.createTimedEventRow(event, now)
		p.listBox.Append(row)
	}
}

// populateAllDayEvents adds the all-day events to the fixed bottom section.
func (p *Popup) populateAllDayEvents(events []calendar.Event, now time.Time) {
	// Header
	header := gtk.NewLabel("All Day")
	header.AddCSSClass("all-day-header")
	header.SetXAlign(0)
	p.allDaySection.Append(header)

	// Event rows
	for _, event := range events {
		row := p.createAllDayEventRow(event, now)
		p.allDaySection.Append(row)
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
	row := gtk.NewBox(gtk.OrientationHorizontal, 0)
	row.AddCSSClass("event-card")

	// Make row clickable to show details
	clickGesture := gtk.NewGestureClick()
	eventCopy := event // Capture for closure
	clickGesture.ConnectReleased(func(nPress int, x, y float64) {
		p.showDetails(eventCopy)
	})
	row.AddController(clickGesture)

	// Time indicator
	timeBox := p.createTimeIndicator(event, now)
	row.Append(timeBox)

	// Event details
	details := gtk.NewBox(gtk.OrientationVertical, 0)
	details.AddCSSClass("event-details")
	details.SetHExpand(true)
	row.Append(details)

	// Title
	title := gtk.NewLabel(event.Summary)
	title.AddCSSClass("event-title")
	title.SetXAlign(0)
	title.SetEllipsize(3) // PANGO_ELLIPSIZE_END
	title.SetMaxWidthChars(35)
	if event.IsOngoing(now) {
		title.AddCSSClass("ongoing")
	}
	details.Append(title)

	// Meta info (duration)
	meta := p.getEventDuration(event)
	if meta != "" {
		metaLabel := gtk.NewLabel(meta)
		metaLabel.AddCSSClass("event-meta")
		metaLabel.SetXAlign(0)
		metaLabel.SetEllipsize(3)
		details.Append(metaLabel)
	}

	// Source
	if event.Source != "" {
		source := gtk.NewLabel(event.Source)
		source.AddCSSClass("event-source")
		source.SetXAlign(0)
		details.Append(source)
	}

	// Join button
	if meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL); meetingLink != "" {
		btn := p.createJoinButton(meetingLink)
		row.Append(btn)
	}

	return row
}

// createAllDayEventRow creates a compact row for an all-day event.
func (p *Popup) createAllDayEventRow(event calendar.Event, now time.Time) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationVertical, 0)
	row.AddCSSClass("all-day-row")

	// Make row clickable to show details
	clickGesture := gtk.NewGestureClick()
	eventCopy := event // Capture for closure
	clickGesture.ConnectReleased(func(nPress int, x, y float64) {
		p.showDetails(eventCopy)
	})
	row.AddController(clickGesture)

	// Title
	title := gtk.NewLabel(event.Summary)
	title.AddCSSClass("all-day-title")
	title.SetXAlign(0)
	title.SetEllipsize(3)
	row.Append(title)

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
		meta.AddCSSClass("all-day-meta")
		meta.SetXAlign(0)
		row.Append(meta)
	}

	return row
}

// createTimeIndicator creates the time display on the left.
func (p *Popup) createTimeIndicator(event calendar.Event, now time.Time) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("time-indicator")
	box.SetVAlign(gtk.AlignCenter)

	var primary, secondary string

	// Convert to local time for display
	localStart := event.Start.Local()

	if event.IsOngoing(now) {
		box.AddCSSClass("now")
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
			box.AddCSSClass("imminent")
			primary = fmt.Sprintf("%dm", int(startsIn.Minutes()))
			secondary = "away"
		}
	}

	primaryLabel := gtk.NewLabel(primary)
	primaryLabel.AddCSSClass("time-primary")
	box.Append(primaryLabel)

	secondaryLabel := gtk.NewLabel(secondary)
	secondaryLabel.AddCSSClass("time-secondary")
	box.Append(secondaryLabel)

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
	btn.AddCSSClass("join-btn")

	// Use icon + label for known services
	box := gtk.NewBox(gtk.OrientationHorizontal, 4)

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
	box.Append(icon)

	label := gtk.NewLabel("Join")
	box.Append(label)

	btn.SetChild(box)

	btn.ConnectClicked(func() {
		slog.Debug("join clicked", "url", meetingLink)
		if p.onJoin != nil {
			p.onJoin(meetingLink)
		} else {
			links.Open(meetingLink)
		}
		p.Hide()
	})

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

	p.statusBar.RemoveCSSClass("stale")

	var text string
	switch {
	case loading:
		text = "Syncing..."
	case stale:
		text = fmt.Sprintf("⚠ Data may be stale • Last sync: %s", lastSync.Format("3:04 PM"))
		p.statusBar.AddCSSClass("stale")
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
	for child := p.detailsView.FirstChild(); child != nil; child = p.detailsView.FirstChild() {
		p.detailsView.Remove(child)
	}

	// Build details header with back button
	detailsHeader := gtk.NewBox(gtk.OrientationHorizontal, 8)
	detailsHeader.AddCSSClass("details-header")

	backBtn := gtk.NewButton()
	backBtn.SetIconName("go-previous-symbolic")
	backBtn.AddCSSClass("details-back-btn")
	backBtn.ConnectClicked(func() {
		p.hideDetails()
	})
	detailsHeader.Append(backBtn)

	headerTitle := gtk.NewLabel("Event Details")
	headerTitle.AddCSSClass("header-title")
	headerTitle.SetHExpand(true)
	headerTitle.SetXAlign(0)
	detailsHeader.Append(headerTitle)

	p.detailsView.Append(detailsHeader)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	p.detailsView.Append(scrolled)

	content := gtk.NewBox(gtk.OrientationVertical, 0)
	content.AddCSSClass("details-content")
	scrolled.SetChild(content)

	// Event title
	title := gtk.NewLabel(event.Summary)
	title.AddCSSClass("details-title")
	title.SetXAlign(0)
	title.SetWrap(true)
	title.SetWrapMode(2) // PANGO_WRAP_WORD_CHAR
	content.Append(title)

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
		descSection := gtk.NewBox(gtk.OrientationVertical, 4)
		descSection.AddCSSClass("details-description-section")

		descLabel := gtk.NewLabel("Description")
		descLabel.AddCSSClass("details-section-label")
		descLabel.SetXAlign(0)
		descSection.Append(descLabel)

		// Strip HTML and clean up description
		cleanDesc := stripHTML(event.Description)
		descText := gtk.NewLabel(cleanDesc)
		descText.AddCSSClass("details-description")
		descText.SetXAlign(0)
		descText.SetWrap(true)
		descText.SetWrapMode(2) // PANGO_WRAP_WORD_CHAR
		descText.SetSelectable(true)
		descSection.Append(descText)

		content.Append(descSection)
	}

	// Join button (if meeting link exists)
	if meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL); meetingLink != "" {
		btnBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
		btnBox.AddCSSClass("details-join-box")
		btnBox.SetHAlign(gtk.AlignCenter)

		joinBtn := p.createJoinButton(meetingLink)
		joinBtn.AddCSSClass("details-join-btn")
		btnBox.Append(joinBtn)

		content.Append(btnBox)
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
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.AddCSSClass("details-row")

	iconLabel := gtk.NewLabel(icon)
	iconLabel.AddCSSClass("details-icon")
	row.Append(iconLabel)

	textLabel := gtk.NewLabel(text)
	textLabel.AddCSSClass("details-text")
	textLabel.SetXAlign(0)
	textLabel.SetWrap(true)
	textLabel.SetWrapMode(2) // PANGO_WRAP_WORD_CHAR
	textLabel.SetSelectable(true)
	row.Append(textLabel)

	container.Append(row)
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
