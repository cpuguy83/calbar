//go:build !nogtk && cgo

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/cpuguy83/calbar/internal/ui"

	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// gtkCallbacks holds stable callback references to avoid exhausting purego callback slots.
type gtkCallbacks struct {
	updateUIOnce sync.Once
	updateUICb   glib.SourceFunc
}

var callbacks gtkCallbacks

// Run starts the application with the appropriate main loop.
func (a *App) Run() error {
	// Check if we're using GTK backend
	backend := a.cfg.UI.Backend
	useGTK := backend == "gtk" || (backend == "auto" || backend == "") && ui.GTKAvailable()

	if useGTK {
		return a.runWithGTK()
	}
	return a.runWithoutGTK()
}

// runWithGTK runs the application with GTK main loop.
func (a *App) runWithGTK() error {
	gtkApp := gtk.NewApplication("com.github.cpuguy83.calbar", gio.GApplicationFlagsNoneValue)

	activateCb := func(app gio.Application) {
		// Hold the application open even without visible windows
		gtkApp.Hold()

		if err := a.activate(); err != nil {
			slog.Error("activation failed", "error", err)
			gtkApp.Quit()
		}
	}
	gtkApp.ConnectActivate(&activateCb)

	// Handle signals to quit GTK gracefully
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("received signal, shutting down")
		if a.cancel != nil {
			a.cancel()
		}
		var cb glib.SourceFunc = func(data uintptr) bool {
			gtkApp.Quit()
			return false
		}
		glib.IdleAdd(&cb, 0)
	}()

	// Run GTK main loop (blocks until app.Quit() is called)
	if code := gtkApp.Run(0, nil); code != 0 {
		return fmt.Errorf("GTK application exited with code %d", code)
	}

	// Cleanup
	a.cleanup()
	return nil
}

// runWithoutGTK runs the application without GTK (menu backend).
func (a *App) runWithoutGTK() error {
	if err := a.activate(); err != nil {
		return fmt.Errorf("activation failed: %w", err)
	}

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("received signal, shutting down")
	a.cleanup()
	return nil
}

// getUpdateUICb returns a stable callback pointer for UI updates.
func (a *App) getUpdateUICb() *glib.SourceFunc {
	callbacks.updateUIOnce.Do(func() {
		callbacks.updateUICb = func(data uintptr) bool {
			a.updateUI()
			return false
		}
	})
	return &callbacks.updateUICb
}

// scheduleUIUpdate schedules a UI update on the appropriate thread.
func (a *App) scheduleUIUpdate() {
	// Check if we're using GTK backend
	backend := a.cfg.UI.Backend
	useGTK := backend == "gtk" || (backend == "auto" || backend == "") && ui.GTKAvailable()

	if useGTK {
		glib.IdleAdd(a.getUpdateUICb(), 0)
	} else {
		// For menu backend, just update directly
		a.updateUI()
	}
}
