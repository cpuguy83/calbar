package main

import (
	"testing"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/config"
)

func TestNotificationTriggers_UsesEventRemindersByDefault(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	trigger := start.Add(-15 * time.Minute)

	a := &App{cfg: &config.Config{Notifications: config.NotificationConfig{Enabled: true}}}
	event := calendar.Event{Start: start, NotifyAt: []time.Time{trigger, trigger}}

	got := a.notificationTriggers(event)
	if len(got) != 1 {
		t.Fatalf("expected 1 deduped trigger, got %d", len(got))
	}
	if !got[0].Equal(trigger) {
		t.Fatalf("unexpected trigger time: got %s want %s", got[0], trigger)
	}
}

func TestNotificationTriggers_BeforeOverridesEventReminders(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	eventTrigger := start.Add(-30 * time.Minute)
	override := 10 * time.Minute

	a := &App{cfg: &config.Config{Notifications: config.NotificationConfig{Enabled: true, Before: []time.Duration{override}}}}
	event := calendar.Event{Start: start, NotifyAt: []time.Time{eventTrigger}}

	got := a.notificationTriggers(event)
	if len(got) != 1 {
		t.Fatalf("expected 1 override trigger, got %d", len(got))
	}

	want := start.Add(-override)
	if !got[0].Equal(want) {
		t.Fatalf("unexpected trigger time: got %s want %s", got[0], want)
	}
}

func TestNotificationTriggers_EmptyBeforeSuppressesEventReminders(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	eventTrigger := start.Add(-30 * time.Minute)

	a := &App{cfg: &config.Config{Notifications: config.NotificationConfig{Enabled: true, Before: []time.Duration{}}}}
	event := calendar.Event{Start: start, NotifyAt: []time.Time{eventTrigger}}

	got := a.notificationTriggers(event)
	if len(got) != 0 {
		t.Fatalf("expected no triggers, got %d", len(got))
	}
}
