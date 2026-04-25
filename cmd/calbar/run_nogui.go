//go:build nogtk || !cgo

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Run starts the application without GTK.
func (a *App) Run() error {
	if err := a.activate(); err != nil {
		return fmt.Errorf("activation failed: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		slog.Info("received signal, shutting down")
		a.Quit()
	case <-a.quitCh:
	}

	a.cleanup()
	return nil
}

// scheduleUIUpdate schedules a UI update.
// Without GTK, we just update directly.
func (a *App) scheduleUIUpdate() {
	a.updateUI()
}
