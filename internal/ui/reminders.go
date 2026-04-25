package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
)

func formatReminderDetails(event calendar.Event, notificationBefore []time.Duration) string {
	if notificationBefore != nil {
		if len(notificationBefore) == 0 {
			return "Notifications disabled for this event by config override"
		}

		triggers := make([]time.Time, 0, len(notificationBefore))
		for _, before := range notificationBefore {
			triggers = append(triggers, event.Start.Add(-before))
		}
		return formatReminderSchedule(triggers, event.Start, true)
	}

	if len(event.NotifyAt) == 0 {
		return "No event-defined reminders"
	}
	return formatReminderSchedule(event.NotifyAt, event.Start, false)
}

func formatReminderSchedule(times []time.Time, eventStart time.Time, overridden bool) string {
	if len(times) == 0 {
		return ""
	}

	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before(sorted[j])
	})

	lines := make([]string, 0, len(sorted)+1)
	if overridden {
		lines = append(lines, "Using config override")
	} else {
		lines = append(lines, "Using event reminders")
	}

	for _, t := range sorted {
		lines = append(lines, fmt.Sprintf("%s (%s)", t.Local().Format("Mon Jan 2, 3:04 PM"), formatReminderOffset(eventStart, t)))
	}

	return strings.Join(lines, "\n")
}

func formatReminderOffset(eventStart, trigger time.Time) string {
	delta := eventStart.Sub(trigger)
	switch {
	case delta == 0:
		return "at start"
	case delta < 0:
		return fmt.Sprintf("%s after start", formatReminderDuration(-delta))
	default:
		return fmt.Sprintf("%s before start", formatReminderDuration(delta))
	}
}

func formatReminderDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		hours := int(d / time.Hour)
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	if d%time.Minute == 0 {
		minutes := int(d / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	return d.Round(time.Second).String()
}

// FormatReminderDetails returns reminder text for callers outside the ui package.
func FormatReminderDetails(event calendar.Event, notificationBefore []time.Duration) string {
	return formatReminderDetails(event, notificationBefore)
}
