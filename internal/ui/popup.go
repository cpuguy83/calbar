// Package ui provides the GTK4 popup window for calbar.
//
// This package requires GTK4 to be installed on the system.
// Build with: go build -tags gtk
package ui

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/links"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Popup is the main popup window showing upcoming events.
type Popup struct {
	window    *gtk.Window
	listBox   *gtk.ListBox
	statusBar *gtk.Label

	mu        sync.RWMutex
	events    []calendar.Event
	timeRange time.Duration
	stale     bool
	lastSync  time.Time
	loading   bool // true until first sync completes

	// Dismiss timer for debounced focus-loss detection
	dismissTimer glib.SourceHandle

	// Callback for meeting join
	onJoin func(url string)
}

// NewPopup creates a new popup window.
func NewPopup(timeRange time.Duration) *Popup {
	p := &Popup{
		timeRange: timeRange,
		loading:   true, // Start in loading state
	}
	return p
}

// Init initializes the GTK widgets. Must be called from GTK main thread.
func (p *Popup) Init() {
	// Create window
	p.window = gtk.NewWindow()
	p.window.SetTitle("CalBar")
	p.window.SetDefaultSize(400, 300)

	// Initialize as layer shell surface if supported (must be before window is realized)
	if gtk4layershell.IsSupported() {
		slog.Debug("layer shell supported, initializing popup as layer surface")

		gtk4layershell.InitForWindow(p.window)
		gtk4layershell.SetLayer(p.window, gtk4layershell.LayerShellLayerTop)
		gtk4layershell.SetAnchor(p.window, gtk4layershell.LayerShellEdgeTop, true)
		gtk4layershell.SetAnchor(p.window, gtk4layershell.LayerShellEdgeRight, true)
		gtk4layershell.SetMargin(p.window, gtk4layershell.LayerShellEdgeTop, 10)
		gtk4layershell.SetMargin(p.window, gtk4layershell.LayerShellEdgeRight, 10)
		gtk4layershell.SetKeyboardMode(p.window, gtk4layershell.LayerShellKeyboardModeOnDemand)
		gtk4layershell.SetNamespace(p.window, "calbar-popup")

		// No decorations for layer shell popup
		p.window.SetDecorated(false)

		// Debounced dismiss on focus loss
		// When window becomes inactive, start a 500ms timer. If still inactive when
		// timer fires, dismiss the popup. If window becomes active again, cancel timer.
		p.window.NotifyProperty("is-active", func() {
			if p.window.IsVisible() {
				if p.window.IsActive() {
					// Window became active - cancel any pending dismiss
					if p.dismissTimer != 0 {
						glib.SourceRemove(p.dismissTimer)
						p.dismissTimer = 0
					}
				} else {
					// Window became inactive - start dismiss timer (unless loading)
					p.mu.RLock()
					loading := p.loading
					p.mu.RUnlock()

					if !loading && p.dismissTimer == 0 {
						p.dismissTimer = glib.TimeoutAdd(500, func() bool {
							// Check if still inactive and visible
							if p.window.IsVisible() && !p.window.IsActive() {
								p.hideAll()
							}
							p.dismissTimer = 0
							return false // Don't repeat
						})
					}
				}
			}
		})
	} else {
		slog.Warn("layer shell not supported, falling back to regular window")
		p.window.SetDecorated(true)
	}

	// Hide instead of destroy on close
	p.window.ConnectCloseRequest(func() bool {
		p.window.SetVisible(false)
		return true // Prevent default destroy
	})

	// Allow Escape key to close the popup
	keyController := gtk.NewEventControllerKey()
	keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			p.hideAll()
			return true
		}
		return false
	})
	p.window.AddController(keyController)

	// Main vertical box
	vbox := gtk.NewBox(gtk.OrientationVertical, 0)
	p.window.SetChild(vbox)

	// Header - use simple label for layer shell, full header bar otherwise
	if gtk4layershell.IsSupported() {
		// Simple title for layer shell popup
		titleBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
		titleBox.AddCSSClass("popup-header")
		titleBox.SetMarginStart(10)
		titleBox.SetMarginEnd(10)
		titleBox.SetMarginTop(10)
		titleBox.SetMarginBottom(5)

		title := gtk.NewLabel("Upcoming Events")
		title.AddCSSClass("title")
		title.SetHExpand(true)
		title.SetXAlign(0)
		titleBox.Append(title)

		vbox.Append(titleBox)
	} else {
		// Full header bar for regular window
		header := gtk.NewHeaderBar()
		header.SetShowTitleButtons(true)
		p.window.SetTitlebar(header)

		title := gtk.NewLabel("Upcoming Events")
		title.AddCSSClass("title")
		header.SetTitleWidget(title)
	}

	// Scrolled window for event list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	vbox.Append(scrolled)

	// Event list
	p.listBox = gtk.NewListBox()
	p.listBox.SetSelectionMode(gtk.SelectionNone)
	p.listBox.AddCSSClass("events-list")
	scrolled.SetChild(p.listBox)

	// Status bar
	p.statusBar = gtk.NewLabel("")
	p.statusBar.AddCSSClass("status-bar")
	p.statusBar.SetXAlign(0)
	p.statusBar.SetMarginStart(10)
	p.statusBar.SetMarginEnd(10)
	p.statusBar.SetMarginTop(5)
	p.statusBar.SetMarginBottom(5)
	vbox.Append(p.statusBar)

	// Apply CSS
	p.applyCSS()

	// Initial empty state
	p.updateList()
}

// applyCSS applies custom styling.
func (p *Popup) applyCSS() {
	css := `
		window {
			border-radius: 8px;
			border: 1px solid alpha(currentColor, 0.2);
		}
		.popup-header {
			border-bottom: 1px solid alpha(currentColor, 0.1);
		}
		.popup-header .title {
			font-weight: bold;
			font-size: 14px;
		}
		.events-list {
			background: transparent;
		}
		.event-row {
			padding: 10px;
			border-bottom: 1px solid alpha(currentColor, 0.1);
		}
		.event-title {
			font-weight: bold;
			font-size: 14px;
		}
		.event-time {
			font-size: 12px;
			opacity: 0.7;
		}
		.event-time.imminent {
			color: @warning_color;
			font-weight: bold;
		}
		.event-source {
			font-size: 11px;
			opacity: 0.5;
		}
		.join-button {
			margin-left: 10px;
		}
		.status-bar {
			font-size: 11px;
			opacity: 0.6;
			border-top: 1px solid alpha(currentColor, 0.1);
		}
		.status-bar.stale {
			color: @warning_color;
		}
	`

	provider := gtk.NewCSSProvider()
	provider.LoadFromData(css)

	display := gdk.DisplayGetDefault()
	if display != nil {
		gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}

// Show shows the popup window.
func (p *Popup) Show() {
	if p.window == nil {
		return
	}
	glib.IdleAdd(func() {
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

// hideAll hides the popup and cancels any pending dismiss timer (must be called from GTK main thread).
func (p *Popup) hideAll() {
	p.window.SetVisible(false)
	// Cancel any pending dismiss timer
	if p.dismissTimer != 0 {
		glib.SourceRemove(p.dismissTimer)
		p.dismissTimer = 0
	}
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
	p.loading = false // First sync complete
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

	// Clear existing items
	for {
		child := p.listBox.FirstChild()
		if child == nil {
			break
		}
		p.listBox.Remove(child)
	}

	p.mu.RLock()
	events := p.events
	timeRange := p.timeRange
	loading := p.loading
	p.mu.RUnlock()

	// Show loading state
	if loading {
		loadingBox := gtk.NewBox(gtk.OrientationVertical, 10)
		loadingBox.SetMarginTop(50)
		loadingBox.SetMarginBottom(50)
		loadingBox.SetHAlign(gtk.AlignCenter)

		spinner := gtk.NewSpinner()
		spinner.Start()
		loadingBox.Append(spinner)

		label := gtk.NewLabel("Loading calendars...")
		label.SetOpacity(0.7)
		loadingBox.Append(label)

		p.listBox.Append(loadingBox)
		p.updateStatusBar()
		return
	}

	now := time.Now()
	cutoff := now.Add(timeRange)

	// Filter and sort events
	var upcoming []calendar.Event
	for _, e := range events {
		// Skip past events (unless ongoing)
		if e.End.Before(now) {
			continue
		}
		// Skip events too far in the future
		if e.Start.After(cutoff) {
			continue
		}
		upcoming = append(upcoming, e)
	}

	// Sort by start time
	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].Start.Before(upcoming[j].Start)
	})

	if len(upcoming) == 0 {
		// Show empty state
		label := gtk.NewLabel("No upcoming events")
		label.SetMarginTop(50)
		label.SetMarginBottom(50)
		label.SetOpacity(0.5)
		p.listBox.Append(label)
	} else {
		for _, event := range upcoming {
			row := p.createEventRow(event)
			p.listBox.Append(row)
		}
	}

	p.updateStatusBar()
}

// createEventRow creates a list row for an event.
func (p *Popup) createEventRow(event calendar.Event) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 10)
	row.AddCSSClass("event-row")

	// Left side: event details
	details := gtk.NewBox(gtk.OrientationVertical, 2)
	details.SetHExpand(true)
	row.Append(details)

	// Title
	title := gtk.NewLabel(event.Summary)
	title.AddCSSClass("event-title")
	title.SetXAlign(0)
	title.SetWrap(false)
	title.SetMaxWidthChars(40)
	details.Append(title)

	// Time
	timeStr := p.formatEventTime(event)
	timeLabel := gtk.NewLabel(timeStr)
	timeLabel.AddCSSClass("event-time")
	timeLabel.SetXAlign(0)

	// Mark as imminent if starting soon
	if event.StartsIn(time.Now()) <= 15*time.Minute && event.StartsIn(time.Now()) > 0 {
		timeLabel.AddCSSClass("imminent")
	}
	details.Append(timeLabel)

	// Source
	if event.Source != "" {
		source := gtk.NewLabel(event.Source)
		source.AddCSSClass("event-source")
		source.SetXAlign(0)
		details.Append(source)
	}

	// Right side: join button if meeting link detected
	meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL)
	if meetingLink != "" {
		service := links.Service(meetingLink)
		joinBtn := gtk.NewButton()
		joinBtn.SetLabel(fmt.Sprintf("Join %s", service))
		joinBtn.AddCSSClass("join-button")
		joinBtn.ConnectClicked(func() {
			slog.Debug("join clicked", "url", meetingLink)
			if p.onJoin != nil {
				p.onJoin(meetingLink)
			} else {
				links.Open(meetingLink)
			}
			// Hide popup after clicking join
			p.Hide()
		})
		row.Append(joinBtn)
	}

	return row
}

// formatEventTime formats the event time for display.
func (p *Popup) formatEventTime(event calendar.Event) string {
	now := time.Now()
	startsIn := event.StartsIn(now)

	// Ongoing event
	if event.IsOngoing(now) {
		remaining := event.End.Sub(now)
		if remaining < time.Hour {
			return fmt.Sprintf("Now (ends in %d min)", int(remaining.Minutes()))
		}
		return fmt.Sprintf("Now (ends in %s)", formatDuration(remaining))
	}

	// Future event
	if startsIn > 0 {
		if startsIn < time.Hour {
			return fmt.Sprintf("in %d minutes", int(startsIn.Minutes()))
		}
		if startsIn < 24*time.Hour {
			return fmt.Sprintf("Today at %s", event.Start.Format("3:04 PM"))
		}
		if startsIn < 48*time.Hour {
			return fmt.Sprintf("Tomorrow at %s", event.Start.Format("3:04 PM"))
		}
		return event.Start.Format("Mon, Jan 2 at 3:04 PM")
	}

	return event.Start.Format("3:04 PM")
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

	var text string
	if loading {
		text = "Syncing calendars..."
		p.statusBar.RemoveCSSClass("stale")
	} else if stale {
		text = fmt.Sprintf("Data may be stale (last sync: %s)", lastSync.Format("3:04 PM"))
		p.statusBar.AddCSSClass("stale")
	} else if lastSync.IsZero() {
		text = "Waiting for sync..."
		p.statusBar.RemoveCSSClass("stale")
	} else {
		text = fmt.Sprintf("%d events - Last sync: %s", eventCount, lastSync.Format("3:04 PM"))
		p.statusBar.RemoveCSSClass("stale")
	}

	p.statusBar.SetText(text)
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}
