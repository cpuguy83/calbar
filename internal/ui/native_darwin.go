//go:build darwin

package ui

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/links"
	"github.com/cpuguy83/calbar/internal/macos"
)

// Native implements the UI interface using the macOS Swift helper.
type Native struct {
	cfg      Config
	frontend *macos.Frontend

	mu           sync.RWMutex
	events       []calendar.Event
	hiddenEvents []calendar.Event
	onAction     func(Action)
}

// NewNative creates a new native macOS UI backend.
func NewNative(cfg Config) UI {
	return &Native{cfg: cfg, frontend: macos.Shared()}
}

// NativeAvailable reports whether the native macOS helper can be found.
func NativeAvailable() bool {
	return true
}

func (n *Native) Init() error {
	n.frontend.OnOpenURL(func(url string) {
		if n.onAction != nil {
			n.onAction(Action{Type: ActionOpenURL, URL: url})
			return
		}
		links.Open(url)
	})
	n.frontend.OnSync(func() {
		if n.onAction != nil {
			n.onAction(Action{Type: ActionSync})
		}
	})
	return nil
}

func (n *Native) Show() {
	_ = n.frontend.Send(macos.Command{Type: "show"})
}

func (n *Native) Hide() {
	_ = n.frontend.Send(macos.Command{Type: "hide"})
}

func (n *Native) Toggle() {
	_ = n.frontend.Send(macos.Command{Type: "toggle"})
}

func (n *Native) Search() {
	_ = n.frontend.Send(macos.Command{Type: "search"})
}

func (n *Native) SetEvents(events []calendar.Event) {
	n.mu.Lock()
	n.events = slices.Clone(events)
	n.mu.Unlock()
	n.sendEvents()
}

func (n *Native) SetHiddenEvents(events []calendar.Event) {
	n.mu.Lock()
	n.hiddenEvents = slices.Clone(events)
	n.mu.Unlock()
}

func (n *Native) SetStale(stale bool) {
	_ = n.frontend.Send(macos.Command{Type: "set_stale", Stale: &stale})
}

func (n *Native) SetLoading(loading bool) {
	_ = n.frontend.Send(macos.Command{Type: "set_loading", Loading: &loading})
}

func (n *Native) SetSyncErrors(messages []string) {
	_ = n.frontend.Send(macos.Command{Type: "set_sync_errors", Errors: slices.Clone(messages)})
}

func (n *Native) OnAction(fn func(Action)) {
	n.onAction = fn
}

func (n *Native) OnHide(fn func(uid string)) {
	n.frontend.OnHide(fn)
}

func (n *Native) OnUnhide(fn func(uid string)) {
	n.frontend.OnUnhide(fn)
}

func (n *Native) sendEvents() {
	n.mu.RLock()
	events := slices.Clone(n.events)
	n.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(n.cfg.TimeRange)
	items := make([]macos.Event, 0, len(events))
	for _, event := range events {
		if event.End.Add(n.cfg.EventEndGrace).Before(now) || event.Start.After(cutoff) {
			continue
		}
		items = append(items, macos.Event{
			UID:             event.UID,
			Summary:         event.Summary,
			Description:     event.Description,
			Section:         nativeEventSection(event, now),
			TimeText:        nativeEventTimeText(event, now),
			TimePrimary:     nativeEventTimePrimary(event, now),
			TimeSecondary:   nativeEventTimeSecondary(event),
			Metadata:        nativeEventMetadata(event),
			Location:        event.Location,
			Organizer:       event.Organizer,
			Source:          event.Source,
			MeetingURL:      nativeMeetingURL(event),
			MeetingService:  event.Meeting.Service,
			MeetingID:       event.Meeting.ID,
			MeetingPasscode: event.Meeting.Passcode,
			MeetingDialIn:   event.Meeting.DialIn,
			MeetingPhoneID:  event.Meeting.PhoneConferenceID,
			AllDay:          event.AllDay,
			Stale:           event.Stale,
		})
	}
	_ = n.frontend.Send(macos.Command{Type: "set_events", Events: items})
}

func nativeEventSection(event calendar.Event, now time.Time) string {
	if event.AllDay {
		return "Today"
	}
	return nativeDayLabel(event.Start, now)
}

func nativeEventTimeText(event calendar.Event, now time.Time) string {
	if event.AllDay {
		return "All day"
	}
	start := event.Start.Local()
	end := event.End.Local()
	day := nativeDayLabel(start, now)
	if event.IsOngoing(now) {
		return fmt.Sprintf("Now until %s", end.Format("3:04 PM"))
	}
	return fmt.Sprintf("%s, %s-%s", day, start.Format("3:04 PM"), end.Format("3:04 PM"))
}

func nativeEventTimePrimary(event calendar.Event, now time.Time) string {
	if event.AllDay {
		return "All"
	}
	if event.IsOngoing(now) {
		return "Now"
	}
	return compactNativeTime(event.Start.Local())
}

func nativeEventTimeSecondary(event calendar.Event) string {
	if event.AllDay {
		return "Day"
	}
	return compactNativeTime(event.End.Local())
}

func compactNativeTime(t time.Time) string {
	return t.Format("3:04p")
}

func nativeEventMetadata(event calendar.Event) string {
	parts := make([]string, 0, 2)
	if event.Source != "" {
		parts = append(parts, event.Source)
	}
	if nativeMeetingURL(event) != "" {
		parts = append(parts, "meeting link")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

func nativeDayLabel(t time.Time, now time.Time) string {
	localTime := t.Local()
	localNow := now.Local()
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	day := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, time.Local)
	switch {
	case day.Equal(today):
		return "Today"
	case day.Equal(today.Add(24 * time.Hour)):
		return "Tomorrow"
	default:
		return localTime.Format("Mon, Jan 2")
	}
}

func nativeMeetingURL(event calendar.Event) string {
	if event.Meeting.URL != "" {
		return event.Meeting.URL
	}
	return links.DetectFromEvent(event.Location, event.Description, event.URL)
}
