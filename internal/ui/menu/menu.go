package menu

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/links"
	"github.com/cpuguy83/calbar/internal/ui"
)

// Config holds menu UI configuration.
type Config struct {
	Program   string   // dmenu program to use (auto-detect if empty)
	Args      []string // extra args to pass to the program
	TimeRange time.Duration
}

// Menu implements the ui.UI interface using dmenu-style launchers.
type Menu struct {
	cfg      Config
	program  string
	onAction func(ui.Action)

	mu        sync.RWMutex
	events    []calendar.Event
	stale     bool
	isShowing bool
}

// New creates a new Menu UI backend.
func New(cfg Config) (*Menu, error) {
	program := cfg.Program
	if program == "" {
		var err error
		program, err = Detect()
		if err != nil {
			return nil, err
		}
		slog.Debug("auto-detected menu program", "program", program)
	} else {
		// Verify the specified program exists
		if _, err := exec.LookPath(program); err != nil {
			return nil, fmt.Errorf("menu program %q not found: %w", program, err)
		}
	}

	return &Menu{
		cfg:     cfg,
		program: program,
	}, nil
}

// Init initializes the menu UI.
func (m *Menu) Init() error {
	return nil // No initialization needed for dmenu
}

// Show displays the event list menu.
func (m *Menu) Show() {
	m.mu.Lock()
	if m.isShowing {
		m.mu.Unlock()
		return
	}
	m.isShowing = true
	events := m.events
	m.mu.Unlock()

	// Run in goroutine to not block
	go func() {
		defer func() {
			m.mu.Lock()
			m.isShowing = false
			m.mu.Unlock()
		}()

		m.showEventList(events)
	}()
}

// Hide closes any open menu.
func (m *Menu) Hide() {
	// dmenu closes itself when user makes a selection or presses Escape
	// Nothing to do here
}

// Toggle shows the menu if not showing, otherwise does nothing.
func (m *Menu) Toggle() {
	m.mu.RLock()
	isShowing := m.isShowing
	m.mu.RUnlock()

	if !isShowing {
		m.Show()
	}
	// Can't programmatically close dmenu, so Toggle just shows
}

// SetEvents updates the event list.
func (m *Menu) SetEvents(events []calendar.Event) {
	m.mu.Lock()
	m.events = events
	m.mu.Unlock()
}

// SetStale marks the data as potentially stale.
func (m *Menu) SetStale(stale bool) {
	m.mu.Lock()
	m.stale = stale
	m.mu.Unlock()
}

// OnAction sets the callback for user actions.
func (m *Menu) OnAction(fn func(ui.Action)) {
	m.onAction = fn
}

// showEventList displays the event list and handles selection.
func (m *Menu) showEventList(events []calendar.Event) {
	lines, eventMap := formatEventList(events, m.cfg.TimeRange)

	selected, err := m.runDmenu(lines, "CalBar")
	if err != nil {
		slog.Debug("menu closed without selection", "error", err)
		return
	}

	selected = strings.TrimSpace(selected)
	slog.Debug("event list selection", "selected", selected, "selectedLen", len(selected), "selectedBytes", fmt.Sprintf("%q", selected))

	if selected == "" || isSeparator(selected) {
		slog.Debug("selection is empty or separator", "isSeparator", isSeparator(selected))
		return
	}

	// Try exact match first
	event, ok := eventMap[selected]
	if !ok {
		// Try to find a matching key (handles potential whitespace differences)
		for key, evt := range eventMap {
			if strings.TrimSpace(key) == selected {
				event = evt
				ok = true
				break
			}
		}
	}

	if !ok {
		slog.Debug("selected item not found in event map", "selected", selected, "mapKeys", eventMapKeys(eventMap))
		return
	}

	slog.Debug("showing details for event", "summary", event.Summary)
	// Show details for selected event
	m.showEventDetails(event, events)
}

// showEventDetails displays event details and handles selection.
func (m *Menu) showEventDetails(event *calendar.Event, allEvents []calendar.Event) {
	lines, urlMap := formatEventDetails(event)

	slog.Debug("showing event details menu", "eventSummary", event.Summary, "lineCount", len(lines))

	selected, err := m.runDmenu(lines, "Details")
	if err != nil {
		slog.Debug("details menu closed without selection", "error", err)
		return
	}

	selected = strings.TrimSpace(selected)
	slog.Debug("details selection", "selected", selected)

	if selected == "" || isSeparator(selected) {
		return
	}

	// Check for back action
	if isBackAction(selected) {
		m.showEventList(allEvents)
		return
	}

	// Check for URL action (urlMap keys are already trimmed)
	if url, ok := urlMap[selected]; ok {
		slog.Debug("opening URL from menu", "url", url)
		if m.onAction != nil {
			m.onAction(ui.Action{Type: ui.ActionOpenURL, URL: url})
		} else {
			links.Open(url)
		}
		return
	}

	// For non-actionable items, copy to clipboard
	copyToClipboard(selected)
}

// runDmenu runs the dmenu program with the given input lines.
// Returns the selected line or an error if the user cancelled.
func (m *Menu) runDmenu(lines []string, prompt string) (string, error) {
	args := m.buildArgs(prompt)
	cmd := exec.Command(m.program, args...)

	// Prepare input
	input := strings.Join(lines, "\n")
	cmd.Stdin = strings.NewReader(input)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("running dmenu", "program", m.program, "args", args)

	if err := cmd.Run(); err != nil {
		// Exit code 1 usually means user cancelled (pressed Escape)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "", fmt.Errorf("cancelled")
			}
		}
		return "", fmt.Errorf("dmenu failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// buildArgs builds command-line arguments for the dmenu program.
func (m *Menu) buildArgs(prompt string) []string {
	var args []string

	switch m.program {
	case "rofi":
		args = []string{"-dmenu", "-p", prompt, "-i"}
	case "wofi":
		args = []string{"--dmenu", "--prompt", prompt, "--insensitive"}
	case "fuzzel":
		args = []string{"--dmenu", "--prompt", prompt + ": "}
	case "bemenu":
		args = []string{"-p", prompt, "-i"}
	case "dmenu":
		args = []string{"-p", prompt, "-i", "-l", "20"}
	default:
		// Generic dmenu-compatible args
		args = []string{"-p", prompt}
	}

	// Add user-specified extra args
	args = append(args, m.cfg.Args...)

	return args
}

// eventMapKeys returns the keys of an event map for debugging.
func eventMapKeys(m map[string]*calendar.Event) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, fmt.Sprintf("%q", k))
	}
	return keys
}

// copyToClipboard copies text to the system clipboard.
// Tries wl-copy (Wayland) first, then xclip (X11).
func copyToClipboard(text string) {
	// Remove leading emoji/icon prefixes for cleaner clipboard content
	clean := strings.TrimSpace(text)
	// Remove common prefixes like "📍 ", "👤 ", "📁 ", etc.
	for _, prefix := range []string{"📍 ", "👤 ", "📁 ", "🔗 "} {
		clean = strings.TrimPrefix(clean, prefix)
	}

	// Try wl-copy first (Wayland)
	if path, err := exec.LookPath("wl-copy"); err == nil && path != "" {
		cmd := exec.Command("wl-copy", clean)
		if err := cmd.Run(); err == nil {
			slog.Debug("copied to clipboard via wl-copy", "text", clean)
			return
		}
	}

	// Fall back to xclip (X11)
	if path, err := exec.LookPath("xclip"); err == nil && path != "" {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(clean)
		if err := cmd.Run(); err == nil {
			slog.Debug("copied to clipboard via xclip", "text", clean)
			return
		}
	}

	// Fall back to xsel (X11)
	if path, err := exec.LookPath("xsel"); err == nil && path != "" {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(clean)
		if err := cmd.Run(); err == nil {
			slog.Debug("copied to clipboard via xsel", "text", clean)
			return
		}
	}

	slog.Debug("no clipboard tool available", "text", clean)
}
