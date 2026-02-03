//go:build !nogtk && cgo

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/cpuguy83/calbar/internal/ui"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

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
	gtkApp := gtk.NewApplication("com.github.cpuguy83.calbar", gio.ApplicationFlagsNone)

	gtkApp.ConnectActivate(func() {
		// Hold the application open even without visible windows
		gtkApp.Hold()

		if err := a.activate(); err != nil {
			slog.Error("activation failed", "error", err)
			gtkApp.Quit()
		}
	})

	// Handle signals to quit GTK gracefully
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("received signal, shutting down")
		if a.cancel != nil {
			a.cancel()
		}
		glib.IdleAdd(func() {
			gtkApp.Quit()
		})
	}()

	// Run GTK main loop (blocks until app.Quit() is called)
	if code := gtkApp.Run(nil); code != 0 {
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

// scheduleUIUpdate schedules a UI update on the appropriate thread.
func (a *App) scheduleUIUpdate() {
	// Check if we're using GTK backend
	backend := a.cfg.UI.Backend
	useGTK := backend == "gtk" || (backend == "auto" || backend == "") && ui.GTKAvailable()

	if useGTK {
		glib.IdleAdd(func() {
			a.updateUI()
		})
	} else {
		// For menu backend, just update directly
		a.updateUI()
	}
}
