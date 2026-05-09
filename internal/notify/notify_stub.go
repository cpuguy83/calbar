//go:build !linux

// Package notify provides desktop notifications.
package notify

import (
	"sync"
	"time"
)

// Notifier sends desktop notifications.
type Notifier struct {
	mu       sync.Mutex
	nextID   uint32
	notified map[string]time.Time
}

// New creates a new notifier.
func New(appName string) (*Notifier, error) {
	return &Notifier{notified: make(map[string]time.Time)}, nil
}

// Close closes the notifier.
func (n *Notifier) Close() error {
	return nil
}

// Notification represents a desktop notification.
type Notification struct {
	Summary string
	Body    string
	Icon    string
	Timeout time.Duration
	Actions []Action
	Urgency Urgency

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

// Send records notification state and returns a synthetic ID.
func (n *Notifier) Send(notif Notification) (uint32, error) {
	if notif.EventUID != "" {
		n.mu.Lock()
		if lastNotified, ok := n.notified[notif.EventUID]; ok && time.Since(lastNotified) < time.Minute {
			n.mu.Unlock()
			return 0, nil
		}
		n.notified[notif.EventUID] = time.Now()
		n.mu.Unlock()
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.nextID++
	return n.nextID, nil
}

// WatchActions listens for notification action invocations.
func (n *Notifier) WatchActions(callback func(id uint32, actionKey string)) error {
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
