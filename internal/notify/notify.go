// Package notify provides desktop notifications via D-Bus.
package notify

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	notifyInterface = "org.freedesktop.Notifications"
	notifyPath      = "/org/freedesktop/Notifications"
)

// Notifier sends desktop notifications via D-Bus.
type Notifier struct {
	conn    *dbus.Conn
	obj     dbus.BusObject
	appName string

	mu       sync.Mutex
	notified map[string]time.Time // Track notified event UIDs to avoid duplicates
}

// New creates a new notifier.
func New(appName string) (*Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	return &Notifier{
		conn:     conn,
		obj:      conn.Object(notifyInterface, notifyPath),
		appName:  appName,
		notified: make(map[string]time.Time),
	}, nil
}

// Close closes the D-Bus connection.
func (n *Notifier) Close() error {
	return n.conn.Close()
}

// Notification represents a desktop notification.
type Notification struct {
	Summary string
	Body    string
	Icon    string
	Timeout time.Duration // 0 = default, -1 = persistent
	Actions []Action
	Urgency Urgency

	// For tracking
	EventUID string
}

// Action represents a notification action button.
type Action struct {
	Key   string
	Label string
}

// Urgency levels for notifications.
type Urgency byte

const (
	UrgencyLow      Urgency = 0
	UrgencyNormal   Urgency = 1
	UrgencyCritical Urgency = 2
)

// Send sends a notification and returns the notification ID.
func (n *Notifier) Send(notif Notification) (uint32, error) {
	// Check if we already notified for this event recently
	if notif.EventUID != "" {
		n.mu.Lock()
		if lastNotified, ok := n.notified[notif.EventUID]; ok {
			// Don't re-notify within 1 minute
			if time.Since(lastNotified) < time.Minute {
				n.mu.Unlock()
				return 0, nil
			}
		}
		n.notified[notif.EventUID] = time.Now()
		n.mu.Unlock()
	}

	// Build actions array: [key1, label1, key2, label2, ...]
	var actions []string
	for _, a := range notif.Actions {
		actions = append(actions, a.Key, a.Label)
	}

	// Build hints map
	hints := map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(byte(notif.Urgency)),
	}

	// Calculate timeout in milliseconds
	timeout := int32(-1) // Use default
	if notif.Timeout > 0 {
		timeout = int32(notif.Timeout.Milliseconds())
	} else if notif.Timeout < 0 {
		timeout = 0 // Persistent
	}

	// Default icon
	icon := notif.Icon
	if icon == "" {
		icon = "x-office-calendar"
	}

	call := n.obj.Call(
		notifyInterface+".Notify",
		0,
		n.appName,     // app_name
		uint32(0),     // replaces_id (0 = new notification)
		icon,          // app_icon
		notif.Summary, // summary
		notif.Body,    // body
		actions,       // actions
		hints,         // hints
		timeout,       // expire_timeout
	)

	if call.Err != nil {
		return 0, fmt.Errorf("send notification: %w", call.Err)
	}

	var id uint32
	if err := call.Store(&id); err != nil {
		return 0, fmt.Errorf("get notification id: %w", err)
	}

	slog.Debug("sent notification", "id", id, "summary", notif.Summary)
	return id, nil
}

// WatchActions listens for notification action invocations.
// The callback receives the notification ID and action key.
func (n *Notifier) WatchActions(callback func(id uint32, actionKey string)) error {
	if err := n.conn.AddMatchSignal(
		dbus.WithMatchInterface(notifyInterface),
		dbus.WithMatchMember("ActionInvoked"),
	); err != nil {
		return fmt.Errorf("add match signal: %w", err)
	}

	ch := make(chan *dbus.Signal, 10)
	n.conn.Signal(ch)

	go func() {
		for sig := range ch {
			if sig.Name != notifyInterface+".ActionInvoked" {
				continue
			}
			if len(sig.Body) < 2 {
				continue
			}

			id, ok1 := sig.Body[0].(uint32)
			key, ok2 := sig.Body[1].(string)
			if ok1 && ok2 {
				callback(id, key)
			}
		}
	}()

	return nil
}

// CleanupOldNotifications removes tracking entries older than maxAge.
func (n *Notifier) CleanupOldNotifications(maxAge time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for uid, t := range n.notified {
		if t.Before(cutoff) {
			delete(n.notified, uid)
		}
	}
}
