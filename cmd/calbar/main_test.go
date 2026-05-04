package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/config"
)

func TestParseCLI(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantConfig  string
		wantVerbose bool
		wantHelp    bool
		wantHelpCmd string
		wantCommand string
		wantArgs    []string
		wantErr     bool
	}{
		{name: "daemon", args: nil},
		{name: "daemon flags", args: []string{"--config", "test.yaml", "-v"}, wantConfig: "test.yaml", wantVerbose: true},
		{name: "base help", args: []string{"--help"}, wantHelp: true},
		{name: "help command", args: []string{"help", "search"}, wantHelp: true, wantHelpCmd: "search"},
		{name: "command", args: []string{"show"}, wantCommand: "show"},
		{name: "global flags before command", args: []string{"--config", "test.yaml", "toggle"}, wantConfig: "test.yaml", wantCommand: "toggle"},
		{name: "command args preserved", args: []string{"search", "-v"}, wantCommand: "search", wantArgs: []string{"-v"}},
		{name: "unknown command", args: []string{"wat"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCLI(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.configPath != tt.wantConfig {
				t.Fatalf("config path got %q, want %q", got.configPath, tt.wantConfig)
			}
			if got.verbose != tt.wantVerbose {
				t.Fatalf("verbose got %v, want %v", got.verbose, tt.wantVerbose)
			}
			if got.help != tt.wantHelp {
				t.Fatalf("help got %v, want %v", got.help, tt.wantHelp)
			}
			if got.helpCommand != tt.wantHelpCmd {
				t.Fatalf("help command got %q, want %q", got.helpCommand, tt.wantHelpCmd)
			}
			if got.command != tt.wantCommand {
				t.Fatalf("command got %q, want %q", got.command, tt.wantCommand)
			}
			if len(got.commandArgs) != len(tt.wantArgs) {
				t.Fatalf("command args got %v, want %v", got.commandArgs, tt.wantArgs)
			}
			for i := range got.commandArgs {
				if got.commandArgs[i] != tt.wantArgs[i] {
					t.Fatalf("command args got %v, want %v", got.commandArgs, tt.wantArgs)
				}
			}
		})
	}
}

func TestParseControlCommand(t *testing.T) {
	help, err := parseControlCommand("show", []string{"--help"})
	if err != nil {
		t.Fatalf("unexpected help error: %v", err)
	}
	if !help {
		t.Fatal("expected help")
	}

	help, err = parseControlCommand("show", []string{"extra"})
	if err == nil {
		t.Fatal("expected error")
	}
	if help {
		t.Fatal("did not expect help")
	}
}

func TestUsageOutput(t *testing.T) {
	var base bytes.Buffer
	printBaseUsage(&base)
	baseText := base.String()
	for _, want := range []string{"Usage:", "Commands:", "calbar [options] <command>", "search"} {
		if !strings.Contains(baseText, want) {
			t.Fatalf("base usage missing %q:\n%s", want, baseText)
		}
	}

	var command bytes.Buffer
	printCommandUsage(&command, "search")
	commandText := command.String()
	for _, want := range []string{"Usage:", "calbar search", "focus search"} {
		if !strings.Contains(commandText, want) {
			t.Fatalf("command usage missing %q:\n%s", want, commandText)
		}
	}
}

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
