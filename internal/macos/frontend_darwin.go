//go:build darwin

// Package macos manages the native macOS helper process.
package macos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

const helperEnv = "CALBAR_MACOS_HELPER"

type Command struct {
	Type         string   `json:"type"`
	State        string   `json:"state,omitempty"`
	Tooltip      string   `json:"tooltip,omitempty"`
	Events       []Event  `json:"events,omitempty"`
	HiddenEvents []Event  `json:"hiddenEvents,omitempty"`
	Loading      *bool    `json:"loading,omitempty"`
	Stale        *bool    `json:"stale,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type Event struct {
	UID             string `json:"uid"`
	Summary         string `json:"summary"`
	Description     string `json:"description,omitempty"`
	Section         string `json:"section,omitempty"`
	TimeText        string `json:"timeText"`
	TimePrimary     string `json:"timePrimary,omitempty"`
	TimeSecondary   string `json:"timeSecondary,omitempty"`
	Metadata        string `json:"metadata,omitempty"`
	Location        string `json:"location,omitempty"`
	Organizer       string `json:"organizer,omitempty"`
	Source          string `json:"source,omitempty"`
	MeetingURL      string `json:"meetingURL,omitempty"`
	MeetingService  string `json:"meetingService,omitempty"`
	MeetingID       string `json:"meetingID,omitempty"`
	MeetingPasscode string `json:"meetingPasscode,omitempty"`
	MeetingDialIn   string `json:"meetingDialIn,omitempty"`
	MeetingPhoneID  string `json:"meetingPhoneID,omitempty"`
	AllDay          bool   `json:"allDay,omitempty"`
	Stale           bool   `json:"stale,omitempty"`
}

type EventMessage struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	UID       string `json:"uid,omitempty"`
	ActionKey string `json:"actionKey,omitempty"`
}

type Frontend struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	encoder *json.Encoder
	started bool

	onActivate       func()
	onCopyConfigPath func()
	onQuit           func()
	onOpenURL        func(string)
	onSync           func()
	onHide           func(string)
	onUnhide         func(string)
}

var shared = &Frontend{}

func Shared() *Frontend {
	return shared
}

func (f *Frontend) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.started {
		return nil
	}

	path, err := helperPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create macOS helper stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("create macOS helper stdout: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("start macOS helper %q: %w", path, err)
	}

	f.cmd = cmd
	f.stdin = stdin
	f.encoder = json.NewEncoder(stdin)
	f.started = true

	go f.readEvents(stdout)
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Warn("macOS helper exited", "error", err)
		}
		f.mu.Lock()
		f.started = false
		f.cmd = nil
		f.stdin = nil
		f.encoder = nil
		f.mu.Unlock()
	}()

	return nil
}

func (f *Frontend) Stop() error {
	f.mu.Lock()
	cmd := f.cmd
	stdin := f.stdin
	f.started = false
	f.cmd = nil
	f.stdin = nil
	f.encoder = nil
	f.mu.Unlock()

	if stdin != nil {
		stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func (f *Frontend) Send(cmd Command) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.started || f.encoder == nil {
		return fmt.Errorf("macOS helper is not running")
	}
	if err := f.encoder.Encode(cmd); err != nil {
		return fmt.Errorf("send macOS helper command %q: %w", cmd.Type, err)
	}
	return nil
}

func (f *Frontend) OnActivate(fn func()) {
	f.mu.Lock()
	f.onActivate = fn
	f.mu.Unlock()
}

func (f *Frontend) OnCopyConfigPath(fn func()) {
	f.mu.Lock()
	f.onCopyConfigPath = fn
	f.mu.Unlock()
}

func (f *Frontend) OnQuit(fn func()) {
	f.mu.Lock()
	f.onQuit = fn
	f.mu.Unlock()
}

func (f *Frontend) OnOpenURL(fn func(string)) {
	f.mu.Lock()
	f.onOpenURL = fn
	f.mu.Unlock()
}

func (f *Frontend) OnSync(fn func()) {
	f.mu.Lock()
	f.onSync = fn
	f.mu.Unlock()
}

func (f *Frontend) OnHide(fn func(string)) {
	f.mu.Lock()
	f.onHide = fn
	f.mu.Unlock()
}

func (f *Frontend) OnUnhide(fn func(string)) {
	f.mu.Lock()
	f.onUnhide = fn
	f.mu.Unlock()
}

func (f *Frontend) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var msg EventMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			slog.Warn("invalid macOS helper event", "error", err)
			continue
		}
		f.dispatch(msg)
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("read macOS helper event", "error", err)
	}
}

func (f *Frontend) dispatch(msg EventMessage) {
	f.mu.Lock()
	onActivate := f.onActivate
	onCopyConfigPath := f.onCopyConfigPath
	onQuit := f.onQuit
	onOpenURL := f.onOpenURL
	onSync := f.onSync
	onHide := f.onHide
	onUnhide := f.onUnhide
	f.mu.Unlock()

	switch msg.Type {
	case "activate":
		if onActivate != nil {
			onActivate()
		}
	case "copy_config_path":
		if onCopyConfigPath != nil {
			onCopyConfigPath()
		}
	case "quit":
		if onQuit != nil {
			onQuit()
		}
	case "open_url":
		if onOpenURL != nil {
			onOpenURL(msg.URL)
		}
	case "sync":
		if onSync != nil {
			onSync()
		}
	case "hide_event":
		if onHide != nil {
			onHide(msg.UID)
		}
	case "unhide_event":
		if onUnhide != nil {
			onUnhide(msg.UID)
		}
	default:
		slog.Warn("unknown macOS helper event", "type", msg.Type)
	}
}

func helperPath() (string, error) {
	if path := os.Getenv(helperEnv); path != "" {
		return path, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable path: %w", err)
	}
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(exeDir, "calbar-macos-helper"),
		filepath.Join(exeDir, "..", "Helpers", "calbar-macos-helper"),
		filepath.Join(exeDir, "..", "MacOS", "calbar-macos-helper"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("macOS helper not found; build cmd/calbar-macos-helper/main.swift and place calbar-macos-helper next to calbar or set %s", helperEnv)
}
