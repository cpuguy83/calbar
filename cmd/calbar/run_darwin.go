//go:build darwin

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Run starts the macOS application.
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
func (a *App) scheduleUIUpdate() {
	a.updateUI()
}
