package menu

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/links"
)

// formatEventList formats events for the main event list menu.
// Returns lines to display and a map of line index -> event for selection handling.
func formatEventList(events, hiddenEvents []calendar.Event, timeRange, eventEndGrace time.Duration) ([]string, map[int]*calendar.Event) {
	now := time.Now()
	cutoff := now.Add(timeRange)
	localNow := now.Local()
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)

	// Separate timed and all-day events
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
			// Only include all-day events that span today
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

	var lines []string
	eventMap := make(map[int]*calendar.Event)
	var lastDay string

	// Add timed events with day separators
	for i := range timedEvents {
		e := &timedEvents[i]
		day := getDayLabel(e.Start, now)
		if day != lastDay {
			lines = append(lines, fmt.Sprintf("━━━━ %s ━━━━", day))
			lastDay = day
		}

		line := formatEventLine(e, now)
		lineIdx := len(lines)
		lines = append(lines, line)
		eventMap[lineIdx] = e
	}

	// Add all-day events section
	if len(allDayEvents) > 0 {
		lines = append(lines, "━━━━ All Day ━━━━")
		for i := range allDayEvents {
			e := &allDayEvents[i]
			prefix := "  "
			if e.Stale {
				prefix = "⚠ "
			}
			line := fmt.Sprintf("%s%s", prefix, e.Summary)
			if e.Source != "" {
				line += fmt.Sprintf(" (%s)", e.Source)
			}
			lineIdx := len(lines)
			lines = append(lines, line)
			eventMap[lineIdx] = e
		}
	}

	// Empty state
	if len(lines) == 0 {
		lines = append(lines, "No upcoming events")
	}

	// Add hidden events indicator if any
	if len(hiddenEvents) > 0 {
		lines = append(lines, "")
		if len(hiddenEvents) == 1 {
			lines = append(lines, "👁 1 hidden event")
		} else {
			lines = append(lines, fmt.Sprintf("👁 %d hidden events", len(hiddenEvents)))
		}
	}

	return lines, eventMap
}

// formatEventLine formats a single timed event for the list.
func formatEventLine(e *calendar.Event, now time.Time) string {
	localStart := e.Start.Local()

	var timeStr string
	if e.IsOngoing(now) {
		remaining := e.End.Sub(now)
		if remaining < time.Hour {
			timeStr = fmt.Sprintf("NOW (%dm left)", int(remaining.Minutes()))
		} else {
			timeStr = fmt.Sprintf("NOW (%.1fh left)", remaining.Hours())
		}
	} else {
		startsIn := e.Start.Sub(now)
		if startsIn <= 15*time.Minute && startsIn > 0 {
			timeStr = fmt.Sprintf("in %dm", int(startsIn.Minutes()))
		} else {
			timeStr = localStart.Format("15:04")
		}
	}

	duration := formatDuration(e.End.Sub(e.Start))
	prefix := "  "
	if e.Stale {
		prefix = "⚠ "
	}
	return fmt.Sprintf("%s%s  %s (%s)", prefix, timeStr, e.Summary, duration)
}

// formatEventDetails formats event details for the details menu.
// Returns lines to display and a map of line -> action URL.
func formatEventDetails(e *calendar.Event) ([]string, map[string]string) {
	now := time.Now()
	localStart := e.Start.Local()
	localEnd := e.End.Local()

	var lines []string
	urlMap := make(map[string]string)

	// Header with event title
	lines = append(lines, fmt.Sprintf("━━━━ %s ━━━━", truncate(e.Summary, 40)))

	// Time info
	if e.AllDay {
		lines = append(lines, fmt.Sprintf("  All Day"))
	} else {
		dayLabel := getDayLabel(e.Start, now)
		timeRange := fmt.Sprintf("%s - %s", localStart.Format("15:04"), localEnd.Format("15:04"))
		duration := formatDuration(e.End.Sub(e.Start))
		lines = append(lines, fmt.Sprintf("  %s, %s (%s)", dayLabel, timeRange, duration))
	}

	// Location
	if e.Location != "" {
		lines = append(lines, fmt.Sprintf("  📍 %s", truncate(e.Location, 50)))
	}

	// Organizer
	if e.Organizer != "" {
		lines = append(lines, fmt.Sprintf("  👤 %s", e.Organizer))
	}

	// Source
	if e.Source != "" {
		lines = append(lines, fmt.Sprintf("  📁 %s", e.Source))
	}

	// Links section
	allLinks := links.DetectAll(e.Location, e.Description, e.URL)
	if len(allLinks) > 0 {
		lines = append(lines, "━━━━ Links ━━━━")
		for _, link := range allLinks {
			line := fmt.Sprintf("  🔗 %s", link.Label)
			lines = append(lines, line)
			// Store with trimmed key since dmenu may strip leading whitespace
			urlMap[strings.TrimSpace(line)] = link.URL
		}
	}

	// Actions
	lines = append(lines, "")
	lines = append(lines, "🚫 Hide this event")
	lines = append(lines, "← Back")

	return lines, urlMap
}

// getDayLabel returns a human-readable day label.
func getDayLabel(t time.Time, now time.Time) string {
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
		return localTime.Format("Mon, Jan 2")
	}
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := d.Hours()
	if hours == float64(int(hours)) {
		return fmt.Sprintf("%dh", int(hours))
	}
	return fmt.Sprintf("%.1fh", hours)
}

// truncate truncates a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// isSeparator returns true if the line is a visual separator (not selectable).
func isSeparator(line string) bool {
	return strings.HasPrefix(line, "━━━━") || line == "" || line == "No upcoming events"
}

// isBackAction returns true if the line is the "Back" action.
func isBackAction(line string) bool {
	return line == "← Back"
}

// isHideAction returns true if the line is the "Hide" action.
func isHideAction(line string) bool {
	return line == "🚫 Hide this event" || strings.Contains(line, "Hide this event")
}

// isHiddenIndicator returns true if the line is the hidden events indicator.
func isHiddenIndicator(line string) bool {
	return strings.HasPrefix(line, "👁 ") && strings.Contains(line, "hidden event")
}

// formatHiddenEvents formats hidden events for the unhide menu.
// Returns lines to display and a map of line -> event for selection handling.
func formatHiddenEvents(hiddenEvents []calendar.Event) ([]string, map[string]*calendar.Event) {
	var lines []string
	eventMap := make(map[string]*calendar.Event)
	now := time.Now()

	lines = append(lines, "━━━━ Hidden Events ━━━━")
	lines = append(lines, "Click to unhide:")
	lines = append(lines, "")

	for i := range hiddenEvents {
		e := &hiddenEvents[i]
		line := formatHiddenEventLine(e, now)
		lines = append(lines, line)
		eventMap[line] = e
	}

	lines = append(lines, "")
	lines = append(lines, "← Back")

	return lines, eventMap
}

// formatHiddenEventLine formats a single hidden event for the list.
func formatHiddenEventLine(e *calendar.Event, now time.Time) string {
	var timeStr string
	if e.AllDay {
		timeStr = "All day"
	} else {
		localStart := e.Start.Local()
		dayLabel := getDayLabel(e.Start, now)
		timeStr = fmt.Sprintf("%s %s", dayLabel, localStart.Format("15:04"))
	}

	return fmt.Sprintf("  %s - %s", timeStr, e.Summary)
}
