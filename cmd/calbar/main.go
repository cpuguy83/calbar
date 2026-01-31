// calbar is a system tray calendar app that displays upcoming events.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	gosync "sync"
	"syscall"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/config"
	"github.com/cpuguy83/calbar/internal/links"
	"github.com/cpuguy83/calbar/internal/notify"
	"github.com/cpuguy83/calbar/internal/sync"
	"github.com/cpuguy83/calbar/internal/tray"
	"github.com/cpuguy83/calbar/internal/ui"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	var (
		configPath = flag.String("config", "", "path to config file (default: ~/.config/calbar/config.yaml)")
		verbose    = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	// Setup logging
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// Load configuration
	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.LoadFrom(*configPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting calbar",
		"interval", cfg.Sync.Interval,
		"time_range", cfg.UI.TimeRange,
	)

	// Create app
	app := &App{
		cfg:            cfg,
		notifiedEvents: make(map[string]time.Time),
	}

	if err := app.Run(); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

// App is the main calbar application.
type App struct {
	cfg      *config.Config
	tray     *tray.Tray
	popup    *ui.Popup
	notifier *notify.Notifier
	syncer   *sync.Syncer

	mu          gosync.RWMutex
	events      []calendar.Event
	lastSync    time.Time
	lastSyncErr error

	// Notification tracking
	notifiedEvents map[string]time.Time

	// Context for background goroutines
	ctx    context.Context
	cancel context.CancelFunc
}

// Run starts the application with GTK main loop.
func (a *App) Run() error {
	// Create GTK application
	gtkApp := gtk.NewApplication("com.github.cpuguy83.calbar", gio.ApplicationFlagsNone)

	gtkApp.ConnectActivate(func() {
		// Hold the application open even without visible windows
		// This is needed for tray apps that don't always have a window shown
		gtkApp.Hold()

		if err := a.activate(); err != nil {
			slog.Error("activation failed", "error", err)
			gtkApp.Quit()
		}
	})

	// Handle signals to quit GTK gracefully
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("received signal, shutting down")
		if a.cancel != nil {
			a.cancel()
		}
		glib.IdleAdd(func() {
			gtkApp.Quit()
		})
	}()

	// Run GTK main loop (blocks until app.Quit() is called)
	if code := gtkApp.Run(nil); code != 0 {
		return fmt.Errorf("GTK application exited with code %d", code)
	}

	// Cleanup
	a.cleanup()
	return nil
}

// activate is called when the GTK application is activated.
func (a *App) activate() error {
	var err error

	// Create context for background goroutines
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// Create syncer
	a.syncer, err = sync.NewSyncer(a.cfg)
	if err != nil {
		return fmt.Errorf("create syncer: %w", err)
	}

	if a.syncer.SourceCount() == 0 {
		return fmt.Errorf("no calendar sources configured")
	}

	// Initialize tray
	a.tray, err = tray.New()
	if err != nil {
		return fmt.Errorf("create tray: %w", err)
	}

	// Create popup
	a.popup = ui.NewPopup(a.cfg.UI.TimeRange)
	a.popup.Init()
	a.popup.OnJoin(func(url string) {
		slog.Debug("opening meeting link", "url", url)
		links.Open(url)
	})

	// Set tray click handler to toggle popup
	a.tray.OnActivate(func() {
		slog.Debug("tray activated, toggling popup")
		a.popup.Toggle()
	})

	if err := a.tray.Start(); err != nil {
		return fmt.Errorf("start tray: %w", err)
	}

	// Initialize notifications
	if a.cfg.Notifications.Enabled {
		a.notifier, err = notify.New("CalBar")
		if err != nil {
			slog.Warn("failed to initialize notifications", "error", err)
		} else {
			// Watch for notification actions
			a.notifier.WatchActions(func(id uint32, actionKey string) {
				slog.Debug("notification action", "id", id, "action", actionKey)
				// TODO: Handle join action - need to track URL per notification ID
			})
		}
	}

	// Start sync goroutine
	go a.syncer.Run(a.ctx, a.onSyncComplete)

	// Start notification checker goroutine
	go a.notificationLoop()

	slog.Info("calbar running",
		"sources", a.syncer.SourceCount(),
		"sync_interval", a.syncer.Interval(),
	)

	return nil
}

// cleanup releases resources when the app is shutting down.
func (a *App) cleanup() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.tray != nil {
		a.tray.Stop()
	}
	if a.notifier != nil {
		a.notifier.Close()
	}
}

// onSyncComplete is called after each sync completes.
func (a *App) onSyncComplete(events []calendar.Event, err error) {
	a.mu.Lock()
	if err != nil {
		slog.Warn("sync failed", "error", err)
		a.lastSyncErr = err
		// Keep old events on error
	} else {
		a.events = events
		a.lastSyncErr = nil
	}
	a.lastSync = time.Now()
	a.mu.Unlock()

	// Update UI on GTK main thread
	glib.IdleAdd(func() {
		a.updateUI()
	})
}

// updateUI updates the popup and tray based on current state.
func (a *App) updateUI() {
	a.mu.RLock()
	events := a.events
	lastSync := a.lastSync
	lastSyncErr := a.lastSyncErr
	a.mu.RUnlock()

	// Update popup with events
	a.popup.SetEvents(events)

	// Update stale state
	isStale := lastSyncErr != nil || time.Since(lastSync) > 2*a.syncer.Interval()
	a.popup.SetStale(isStale)

	// Update tray state
	if isStale {
		a.tray.SetState(tray.StateStale)
	} else {
		// Check for imminent events
		a.updateTrayState()
	}

	// Update tooltip
	a.updateTrayTooltip()
}

// updateTrayState updates the tray icon based on upcoming events.
func (a *App) updateTrayState() {
	a.mu.RLock()
	events := a.events
	a.mu.RUnlock()

	now := time.Now()

	for _, e := range events {
		if e.End.Before(now) {
			continue
		}

		startsIn := e.Start.Sub(now)
		if startsIn > 0 && startsIn <= 15*time.Minute {
			a.tray.SetState(tray.StateImminent)
			return
		}
	}

	a.tray.SetState(tray.StateNormal)
}

// updateTrayTooltip updates the tray tooltip with the next event.
func (a *App) updateTrayTooltip() {
	a.mu.RLock()
	events := a.events
	a.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(a.cfg.UI.TimeRange)

	for _, e := range events {
		if e.End.Before(now) {
			continue
		}
		if e.Start.After(cutoff) {
			continue
		}

		startsIn := e.Start.Sub(now)
		var timeStr string
		if startsIn < 0 {
			timeStr = "Now"
		} else if startsIn < time.Hour {
			timeStr = fmt.Sprintf("in %d min", int(startsIn.Minutes()))
		} else {
			timeStr = e.Start.Format("3:04 PM")
		}

		a.tray.SetTooltip(fmt.Sprintf("%s - %s", e.Summary, timeStr))
		return
	}

	a.tray.SetTooltip("No upcoming events")
}

// notificationLoop checks for upcoming events and sends notifications.
func (a *App) notificationLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.checkNotifications()
			// Also update tray state periodically
			glib.IdleAdd(func() {
				a.updateTrayState()
			})
		case <-a.ctx.Done():
			return
		}
	}
}

// checkNotifications sends notifications for upcoming events.
func (a *App) checkNotifications() {
	if a.notifier == nil || !a.cfg.Notifications.Enabled {
		return
	}

	a.mu.RLock()
	events := a.events
	a.mu.RUnlock()

	now := time.Now()

	for _, e := range events {
		if e.End.Before(now) {
			continue
		}

		startsIn := e.Start.Sub(now)
		if startsIn < 0 {
			continue
		}

		// Check each notification threshold
		for _, before := range a.cfg.Notifications.Before {
			// Check if we're within the notification window
			if startsIn <= before && startsIn > before-time.Minute {
				key := e.UID + "-" + before.String()

				// Check if already notified
				if _, ok := a.notifiedEvents[key]; ok {
					continue
				}

				a.sendNotification(e, startsIn)
				a.notifiedEvents[key] = now
			}
		}
	}

	// Cleanup old entries
	cutoff := now.Add(-24 * time.Hour)
	for k, t := range a.notifiedEvents {
		if t.Before(cutoff) {
			delete(a.notifiedEvents, k)
		}
	}
}

// sendNotification sends a notification for an event.
func (a *App) sendNotification(event calendar.Event, startsIn time.Duration) {
	body := fmt.Sprintf("Starts in %s", formatDuration(startsIn))

	notif := notify.Notification{
		Summary:  event.Summary,
		Body:     body,
		EventUID: event.UID,
		Urgency:  notify.UrgencyNormal,
	}

	// Add join action if meeting link detected
	meetingLink := links.DetectFromEvent(event.Location, event.Description, event.URL)
	if meetingLink != "" {
		notif.Actions = []notify.Action{
			{Key: "join", Label: "Join Meeting"},
		}
	}

	if startsIn <= 5*time.Minute {
		notif.Urgency = notify.UrgencyCritical
	}

	if _, err := a.notifier.Send(notif); err != nil {
		slog.Warn("failed to send notification", "error", err)
	}
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "now"
	}
	if d < time.Minute {
		return "< 1 min"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}
