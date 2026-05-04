// calbar is a system tray calendar app that displays upcoming events.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	gosync "sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/clipboard"
	"github.com/cpuguy83/calbar/internal/config"
	"github.com/cpuguy83/calbar/internal/links"
	"github.com/cpuguy83/calbar/internal/notify"
	"github.com/cpuguy83/calbar/internal/sync"
	"github.com/cpuguy83/calbar/internal/tray"
	"github.com/cpuguy83/calbar/internal/ui"
	"github.com/cpuguy83/calbar/internal/ui/menu"
)

// hiddenEntry tracks a hidden event UID and when it was hidden.
type hiddenEntry struct {
	uid    string
	hidden time.Time
}

func main() {
	cli, err := parseCLI(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "calbar: %v\n\n", err)
		printBaseUsage(os.Stderr)
		os.Exit(2)
	}
	if cli.help {
		printUsage(os.Stdout, cli.helpCommand)
		return
	}

	if cli.command != "" {
		help, err := parseControlCommand(cli.command, cli.commandArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calbar %s: %v\n\n", cli.command, err)
			printCommandUsage(os.Stderr, cli.command)
			os.Exit(2)
		}
		if help {
			printCommandUsage(os.Stdout, cli.command)
			return
		}

		setupLogging(cli.verbose)
		if err := sendControlCommand(cli.command); err != nil {
			slog.Error("command failed", "command", cli.command, "error", err)
			os.Exit(1)
		}
		return
	}

	setupLogging(cli.verbose)
	if err := runDaemon(cli.configPath); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

type cliOptions struct {
	configPath  string
	verbose     bool
	help        bool
	helpCommand string
	command     string
	commandArgs []string
}

func parseCLI(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("calbar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath, verbose, help := addBaseFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	cli := cliOptions{configPath: *configPath, verbose: *verbose, help: *help}
	remaining := fs.Args()
	if cli.help {
		return cli, nil
	}
	if len(remaining) == 0 {
		return cli, nil
	}
	if remaining[0] == "help" {
		cli.help = true
		if len(remaining) > 2 {
			return cliOptions{}, fmt.Errorf("help takes at most one command")
		}
		if len(remaining) == 2 {
			if _, ok := controlCommandMethods[remaining[1]]; !ok {
				return cliOptions{}, fmt.Errorf("unknown command %q", remaining[1])
			}
			cli.helpCommand = remaining[1]
		}
		return cli, nil
	}
	cli.command = remaining[0]
	cli.commandArgs = remaining[1:]
	if _, ok := controlCommandMethods[cli.command]; !ok {
		return cliOptions{}, fmt.Errorf("unknown command %q", cli.command)
	}
	return cli, nil
}

func parseControlCommand(command string, args []string) (bool, error) {
	fs := flag.NewFlagSet("calbar "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := addHelpFlags(fs)
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if *help {
		return true, nil
	}
	if fs.NArg() != 0 {
		return false, fmt.Errorf("%s takes no arguments", command)
	}
	return false, nil
}

func addBaseFlags(fs *flag.FlagSet) (*string, *bool, *bool) {
	configPath := fs.String("config", "", "path to config file (default: ~/.config/calbar/config.yaml)")
	verbose := fs.Bool("v", false, "verbose logging")
	help := addHelpFlags(fs)
	return configPath, verbose, help
}

func addHelpFlags(fs *flag.FlagSet) *bool {
	help := fs.Bool("help", false, "show help")
	fs.BoolFunc("h", "show help", func(string) error {
		*help = true
		return nil
	})
	return help
}

func printUsage(w io.Writer, command string) {
	if command != "" {
		printCommandUsage(w, command)
		return
	}
	printBaseUsage(w)
}

func printBaseUsage(w io.Writer) {
	fmt.Fprintln(w, "CalBar displays upcoming calendar events in a tray popup or menu launcher.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  calbar [options]")
	fmt.Fprintln(w, "  calbar [options] <command> [command options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, name := range controlCommandNames {
		fmt.Fprintf(w, "  %-8s %s\n", name, controlCommandDescriptions[name])
	}
	fmt.Fprintln(w, "  help     Show help for calbar or a command")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  -config string")
	fmt.Fprintln(w, "        path to config file (default: ~/.config/calbar/config.yaml)")
	fmt.Fprintln(w, "  -v")
	fmt.Fprintln(w, "        verbose logging")
	fmt.Fprintln(w, "  -h, -help")
	fmt.Fprintln(w, "        show help")
}

func printCommandUsage(w io.Writer, command string) {
	desc := controlCommandDescriptions[command]
	if desc == "" {
		desc = "Control the active CalBar instance"
	}
	fmt.Fprintf(w, "Usage:\n  calbar %s [options]\n\n", command)
	fmt.Fprintln(w, desc+".")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  -h, -help")
	fmt.Fprintln(w, "        show help")
}

func runDaemon(configPath string) error {
	// Load configuration
	var cfg *config.Config
	var resolvedConfigPath string
	var err error
	if configPath != "" {
		resolvedConfigPath, err = filepath.Abs(config.ResolvePath(configPath))
		if err != nil {
			return fmt.Errorf("resolve config path %q: %w", configPath, err)
		}
		cfg, err = config.LoadFrom(configPath)
	} else {
		resolvedConfigPath, err = config.DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve default config path: %w", err)
		}
		cfg, err = config.Load()
	}
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	slog.Info("starting calbar",
		"interval", cfg.Sync.Interval,
		"time_range", cfg.UI.TimeRange,
		"backend", cfg.UI.Backend,
	)

	// Create app
	app := &App{
		cfg:             cfg,
		configPath:      resolvedConfigPath,
		quitCh:          make(chan struct{}),
		notifiedEvents:  make(map[string]time.Time),
		notificationIDs: make(map[uint32]string),
	}

	return app.Run()
}

func setupLogging(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
}

// App is the main calbar application.
type App struct {
	cfg        *config.Config
	configPath string
	tray       *tray.Tray
	ui         ui.UI
	notifier   *notify.Notifier
	syncer     *sync.Syncer
	control    *controlServer

	mu            gosync.RWMutex
	events        []calendar.Event
	hiddenEntries []hiddenEntry // UIDs hidden by user, sorted by hide time (oldest first)
	lastSync      time.Time
	lastSyncErr   error
	syncing       bool

	// Notification tracking
	notifiedEvents  map[string]time.Time
	notificationIDs map[uint32]string // notification ID -> meeting URL

	// Context for background goroutines
	ctx    context.Context
	cancel context.CancelFunc

	quitOnce gosync.Once
	quitCh   chan struct{}
	quitFn   func()
}

// selectBackend determines which UI backend to use based on config.
func (a *App) selectBackend() (ui.UI, error) {
	backend := a.cfg.UI.Backend

	switch backend {
	case "gtk":
		if !ui.GTKAvailable() {
			return nil, fmt.Errorf("GTK requested but not available (build with CGO and GTK libraries)")
		}
		slog.Info("using GTK backend")
		return ui.NewGTK(ui.Config{
			TimeRange:          a.cfg.UI.TimeRange,
			EventEndGrace:      a.cfg.UI.EventEndGrace,
			HoverDismissDelay:  *a.cfg.UI.HoverDismissDelay,
			NotificationBefore: a.cfg.Notifications.Before,
			CSSFile:            a.cfg.UI.CSSFile,
		}), nil

	case "menu":
		slog.Info("using menu backend")
		return menu.New(menu.Config{
			Program:            a.cfg.UI.Menu.Program,
			Args:               a.cfg.UI.Menu.Args,
			TimeRange:          a.cfg.UI.TimeRange,
			EventEndGrace:      a.cfg.UI.EventEndGrace,
			NotificationBefore: a.cfg.Notifications.Before,
		})

	case "auto", "":
		// Auto: prefer GTK if available, fall back to menu
		if ui.GTKAvailable() {
			slog.Info("auto-selected GTK backend")
			return ui.NewGTK(ui.Config{
				TimeRange:          a.cfg.UI.TimeRange,
				EventEndGrace:      a.cfg.UI.EventEndGrace,
				HoverDismissDelay:  *a.cfg.UI.HoverDismissDelay,
				NotificationBefore: a.cfg.Notifications.Before,
				CSSFile:            a.cfg.UI.CSSFile,
			}), nil
		}
		slog.Info("GTK not available, falling back to menu backend")
		return menu.New(menu.Config{
			Program:            a.cfg.UI.Menu.Program,
			Args:               a.cfg.UI.Menu.Args,
			TimeRange:          a.cfg.UI.TimeRange,
			EventEndGrace:      a.cfg.UI.EventEndGrace,
			NotificationBefore: a.cfg.Notifications.Before,
		})

	default:
		return nil, fmt.Errorf("unknown UI backend: %s", backend)
	}
}

// activate initializes all app components.
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

	// Select and create UI backend
	a.ui, err = a.selectBackend()
	if err != nil {
		return fmt.Errorf("create UI backend: %w", err)
	}

	// Initialize UI
	if err := a.ui.Init(); err != nil {
		return fmt.Errorf("init UI: %w", err)
	}

	a.control, err = startControlServer(a)
	if err != nil {
		return fmt.Errorf("start control server: %w", err)
	}

	// Set up action handler
	a.ui.OnAction(func(action ui.Action) {
		switch action.Type {
		case ui.ActionOpenURL:
			slog.Debug("opening URL", "url", action.URL)
			links.Open(action.URL)
		case ui.ActionSync:
			a.triggerSync()
		}
	})

	// Set up hide handler
	a.ui.OnHide(func(uid string) {
		a.hideEvent(uid)
	})

	// Set up unhide handler
	a.ui.OnUnhide(func(uid string) {
		a.unhideEvent(uid)
	})

	// Set tray click handler to toggle UI
	a.tray.OnActivate(func() {
		slog.Debug("tray activated, toggling UI")
		a.ui.Toggle()
	})
	a.tray.OnCopyConfigPath(func() {
		if err := a.CopyConfigPath(); err != nil {
			slog.Warn("failed to copy config path", "path", a.configPath, "error", err)
		}
	})
	a.tray.OnQuit(func() {
		a.Quit()
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
			// Watch for notification actions (e.g., "Join Meeting" button)
			a.notifier.WatchActions(func(id uint32, actionKey string) {
				if actionKey == "join" {
					a.mu.RLock()
					url := a.notificationIDs[id]
					a.mu.RUnlock()

					if url != "" {
						slog.Debug("opening meeting from notification", "url", url)
						links.Open(url)
					}
				}
			})
		}
	}

	// Start sync goroutine
	go func() {
		a.triggerSync()
		ticker := time.NewTicker(a.syncer.Interval())
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				a.triggerSync()
			case <-a.ctx.Done():
				return
			}
		}
	}()

	// Start notification checker goroutine
	go a.notificationLoop()

	slog.Info("calbar running",
		"sources", a.syncer.SourceCount(),
		"sync_interval", a.syncer.Interval(),
	)

	return nil
}

// CopyConfigPath copies the resolved config path to the clipboard.
func (a *App) CopyConfigPath() error {
	if a.configPath == "" {
		return fmt.Errorf("config path is not set")
	}
	return clipboard.Copy(a.configPath)
}

// Quit requests application shutdown through the active runtime loop.
func (a *App) Quit() {
	a.quitOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		if a.quitFn != nil {
			a.quitFn()
		}
		close(a.quitCh)
	})
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
	if a.control != nil {
		a.control.Close()
	}
}

// hideEvent hides an event by UID (ephemeral, until restart).
func (a *App) hideEvent(uid string) {
	a.mu.Lock()
	// Check if already hidden to avoid duplicates
	alreadyHidden := false
	for _, e := range a.hiddenEntries {
		if e.uid == uid {
			alreadyHidden = true
			break
		}
	}
	if !alreadyHidden {
		a.hiddenEntries = append(a.hiddenEntries, hiddenEntry{uid: uid, hidden: time.Now()})
	}
	a.gcHiddenEntries()
	a.mu.Unlock()

	slog.Debug("event hidden", "uid", uid)
	a.scheduleUIUpdate()
}

// unhideEvent removes an event from the hidden list.
func (a *App) unhideEvent(uid string) {
	a.mu.Lock()
	a.hiddenEntries = slices.DeleteFunc(a.hiddenEntries, func(e hiddenEntry) bool {
		return e.uid == uid
	})
	a.gcHiddenEntries()
	a.mu.Unlock()

	slog.Debug("event unhidden", "uid", uid)
	a.scheduleUIUpdate()
}

// visibleEvents returns events that are not hidden by the user.
// Must be called with at least RLock held.
func (a *App) visibleEvents() []calendar.Event {
	if len(a.hiddenEntries) == 0 {
		return a.events
	}
	// Build a set of hidden UIDs for O(1) lookup
	hiddenSet := make(map[string]struct{}, len(a.hiddenEntries))
	for _, h := range a.hiddenEntries {
		hiddenSet[h.uid] = struct{}{}
	}
	visible := make([]calendar.Event, 0, len(a.events))
	for _, e := range a.events {
		if _, hidden := hiddenSet[e.UID]; !hidden {
			visible = append(visible, e)
		}
	}
	return visible
}

// hiddenEvents returns events that are hidden by the user, sorted by hide time (most recent first).
// Must be called with at least RLock held.
func (a *App) hiddenEvents() []calendar.Event {
	if len(a.hiddenEntries) == 0 {
		return nil
	}

	// Build a map of UID -> event for quick lookup
	eventByUID := make(map[string]calendar.Event, len(a.events))
	for _, e := range a.events {
		eventByUID[e.UID] = e
	}

	// Iterate in reverse order (newest first) since slice is sorted oldest-first
	var result []calendar.Event
	for _, entry := range slices.Backward(a.hiddenEntries) {
		if e, ok := eventByUID[entry.uid]; ok {
			result = append(result, e)
		}
	}
	return result
}

// gcHiddenEntries removes hidden entries for events that are no longer visible
// (either removed by sync or past their end time + grace period).
// Must be called with Lock held.
func (a *App) gcHiddenEntries() {
	if len(a.hiddenEntries) == 0 {
		return
	}

	// Build a map of UID -> event for quick lookup
	eventByUID := make(map[string]calendar.Event, len(a.events))
	for _, e := range a.events {
		eventByUID[e.UID] = e
	}

	now := time.Now()
	eventEndGrace := a.cfg.UI.EventEndGrace

	a.hiddenEntries = slices.DeleteFunc(a.hiddenEntries, func(h hiddenEntry) bool {
		e, exists := eventByUID[h.uid]
		if !exists {
			// Event no longer in sync results
			return true
		}
		if e.End.Add(eventEndGrace).Before(now) {
			// Event has ended
			return true
		}
		return false
	})
}

func (a *App) beginSync() bool {
	a.mu.Lock()
	if a.syncing {
		a.mu.Unlock()
		return false
	}
	a.syncing = true
	a.mu.Unlock()
	a.ui.SetLoading(true)
	return true
}

func (a *App) endSync() {
	a.mu.Lock()
	a.syncing = false
	a.mu.Unlock()
	a.ui.SetLoading(false)
}

func (a *App) triggerSync() {
	if !a.beginSync() {
		slog.Debug("sync request ignored; already syncing")
		return
	}

	go func() {
		events, failedSources, err := a.syncer.Sync(a.ctx)
		a.onSyncComplete(events, failedSources, err)
	}()
}

// onSyncComplete is called after each sync completes.
func (a *App) onSyncComplete(events []calendar.Event, failedSources []string, err error) {
	a.mu.Lock()
	if err != nil {
		slog.Warn("sync failed", "error", err)
		a.lastSyncErr = err
		// Keep old events on complete failure
	} else {
		// Build set of failed sources for quick lookup
		failedSet := make(map[string]bool, len(failedSources))
		for _, s := range failedSources {
			failedSet[s] = true
		}

		// Keep old events from failed sources, marking them as stale
		var merged []calendar.Event
		for _, e := range a.events {
			if failedSet[e.Source] {
				e.Stale = true
				merged = append(merged, e)
			}
		}

		// Add new events from successful sources (not stale)
		for i := range events {
			events[i].Stale = false
		}
		merged = append(merged, events...)

		// Merge and sort
		a.events = calendar.Merge(merged)
		a.lastSyncErr = nil

		if len(failedSources) > 0 {
			slog.Warn("some sources failed, keeping stale events", "failed_sources", failedSources)
		}
	}
	a.lastSync = time.Now()
	a.mu.Unlock()
	a.endSync()

	// Update UI - schedule on appropriate thread
	a.scheduleUIUpdate()
}

// updateUI updates the UI and tray based on current state.
func (a *App) updateUI() {
	a.mu.RLock()
	events := a.visibleEvents()
	hidden := a.hiddenEvents()
	lastSync := a.lastSync
	lastSyncErr := a.lastSyncErr
	a.mu.RUnlock()

	// Update UI with events
	a.ui.SetEvents(events)
	a.ui.SetHiddenEvents(hidden)

	// Update stale state
	isStale := lastSyncErr != nil || time.Since(lastSync) > 2*a.syncer.Interval()
	a.ui.SetStale(isStale)

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
	events := a.visibleEvents()
	a.mu.RUnlock()

	now := time.Now()
	eventEndGrace := a.cfg.UI.EventEndGrace

	for _, e := range events {
		// Keep events visible for a grace period after they end
		if e.End.Add(eventEndGrace).Before(now) {
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
	events := a.visibleEvents()
	a.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(a.cfg.UI.TimeRange)
	eventEndGrace := a.cfg.UI.EventEndGrace

	for _, e := range events {
		// Skip all-day events for tooltip
		if e.AllDay {
			continue
		}
		// Keep events visible for a grace period after they end
		if e.End.Add(eventEndGrace).Before(now) {
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
			a.scheduleUIUpdate()
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
	events := a.visibleEvents()
	a.mu.RUnlock()

	now := time.Now()
	eventEndGrace := a.cfg.UI.EventEndGrace

	for _, e := range events {
		// Keep events visible for a grace period after they end
		if e.End.Add(eventEndGrace).Before(now) {
			continue
		}

		for _, trigger := range a.notificationTriggers(e) {
			if trigger.Before(now) || trigger.After(now.Add(time.Minute)) {
				continue
			}

			key := fmt.Sprintf("%s-%d", e.UID, trigger.Unix())
			if _, ok := a.notifiedEvents[key]; ok {
				continue
			}

			startsIn := e.Start.Sub(now)
			a.sendNotification(e, startsIn)
			a.notifiedEvents[key] = now
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

func (a *App) notificationTriggers(event calendar.Event) []time.Time {
	if a.cfg.Notifications.Before != nil {
		triggers := make([]time.Time, 0, len(a.cfg.Notifications.Before))
		for _, before := range a.cfg.Notifications.Before {
			triggers = append(triggers, event.Start.Add(-before))
		}
		return dedupeTimes(triggers)
	}

	return dedupeTimes(event.NotifyAt)
}

func dedupeTimes(times []time.Time) []time.Time {
	if len(times) < 2 {
		return times
	}

	clone := append([]time.Time(nil), times...)
	sort.Slice(clone, func(i, j int) bool {
		return clone[i].Before(clone[j])
	})

	result := clone[:0]
	for _, t := range clone {
		if len(result) > 0 && result[len(result)-1].Equal(t) {
			continue
		}
		result = append(result, t)
	}
	return result
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

	id, err := a.notifier.Send(notif)
	if err != nil {
		slog.Warn("failed to send notification", "error", err)
		return
	}

	// Track notification ID -> meeting URL for join action
	if meetingLink != "" && id != 0 {
		a.mu.Lock()
		a.notificationIDs[id] = meetingLink
		a.mu.Unlock()
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
