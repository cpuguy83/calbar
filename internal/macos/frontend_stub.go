//go:build !darwin

// Package macos manages the native macOS helper process.
package macos

import "fmt"

type Command struct {
	Type    string
	State   string
	Tooltip string
	Events  []Event
	Loading *bool
	Stale   *bool
	Errors  []string
}

type Event struct {
	UID        string
	Summary    string
	TimeText   string
	Location   string
	Source     string
	MeetingURL string
	AllDay     bool
	Stale      bool
}

type Frontend struct{}

func Shared() *Frontend {
	return &Frontend{}
}

func HelperAvailable() bool {
	return false
}

func (f *Frontend) Start() error {
	return fmt.Errorf("macOS helper is not available on this platform")
}

func (f *Frontend) Stop() error {
	return nil
}

func (f *Frontend) Send(cmd Command) error {
	return fmt.Errorf("macOS helper is not available on this platform")
}

func (f *Frontend) OnActivate(fn func()) {}

func (f *Frontend) OnCopyConfigPath(fn func()) {}

func (f *Frontend) OnQuit(fn func()) {}

func (f *Frontend) OnOpenURL(fn func(string)) {}

func (f *Frontend) OnSync(fn func()) {}

func (f *Frontend) OnHide(fn func(string)) {}

func (f *Frontend) OnUnhide(fn func(string)) {}
