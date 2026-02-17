// Package sync provides calendar synchronization from multiple sources.
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
	"github.com/cpuguy83/calbar/internal/config"
	"github.com/cpuguy83/calbar/internal/filter"
)

// sourceWithFilter pairs a calendar source with its optional filter.
type sourceWithFilter struct {
	source calendar.Source
	filter *filter.Filter
}

// Syncer handles calendar synchronization from multiple sources.
type Syncer struct {
	sources   []sourceWithFilter
	interval  time.Duration
	timeRange time.Duration
}

// NewSyncer creates a new Syncer from configuration.
func NewSyncer(cfg *config.Config) (*Syncer, error) {
	sources, err := createSources(cfg.Sources)
	if err != nil {
		return nil, err
	}

	return &Syncer{
		sources:   sources,
		interval:  cfg.Sync.Interval,
		timeRange: cfg.Sync.TimeRange,
	}, nil
}

// Interval returns the configured sync interval.
func (s *Syncer) Interval() time.Duration {
	return s.interval
}

// SourceCount returns the number of configured sources.
func (s *Syncer) SourceCount() int {
	return len(s.sources)
}

// Sync fetches all sources, applies per-source filters, and returns merged events.
// Also returns the names of any sources that failed to sync.
func (s *Syncer) Sync(ctx context.Context) ([]calendar.Event, []string, error) {
	slog.Info("starting sync", "sources", len(s.sources))

	// Calculate end time from configured time range
	endTime := time.Now().Add(s.timeRange)

	// Fetch from all sources in parallel, applying per-source filters
	type result struct {
		events   []calendar.Event
		name     string
		fetched  int // count before filtering
		filtered int // count after filtering
		err      error
	}

	results := make(chan result, len(s.sources))
	var wg sync.WaitGroup

	for _, swf := range s.sources {
		wg.Go(func() {
			name := swf.source.Name()
			slog.Debug("fetching source", "name", name)

			events, err := swf.source.Fetch(ctx, endTime)
			if err != nil {
				results <- result{name: name, err: err}
				return
			}

			fetched := len(events)

			// Apply per-source filter (if no rules, all events pass through)
			if swf.filter != nil {
				events = swf.filter.Apply(events)
			}

			results <- result{
				events:   events,
				name:     name,
				fetched:  fetched,
				filtered: len(events),
				err:      nil,
			}
		})
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allEvents []calendar.Event
	var failedSources []string
	var firstErr error
	for r := range results {
		if r.err != nil {
			slog.Warn("failed to fetch source", "name", r.name, "error", r.err)
			failedSources = append(failedSources, r.name)
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		slog.Info("fetched source", "name", r.name, "fetched", r.fetched, "after_filter", r.filtered)
		allEvents = append(allEvents, r.events...)
	}

	// Merge and sort
	merged := calendar.Merge(allEvents)

	slog.Info("sync complete", "events", len(merged), "failed_sources", len(failedSources))

	// Return events even if some sources failed (partial success)
	// Only return error if we got zero events and there was an error
	if len(merged) == 0 && firstErr != nil {
		return nil, failedSources, firstErr
	}

	return merged, failedSources, nil
}

// Run starts the sync loop, calling onSync after each sync completes.
// The callback receives the synced events, names of failed sources, and any error.
// Run blocks until the context is cancelled.
func (s *Syncer) Run(ctx context.Context, onSync func([]calendar.Event, []string, error)) {
	// Initial sync
	events, failedSources, err := s.Sync(ctx)
	onSync(events, failedSources, err)

	// Periodic sync
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			events, failedSources, err := s.Sync(ctx)
			onSync(events, failedSources, err)
		case <-ctx.Done():
			return
		}
	}
}

// createSources creates calendar sources with their per-source filters from configuration.
func createSources(cfgs []config.SourceConfig) ([]sourceWithFilter, error) {
	var sources []sourceWithFilter

	for _, cfg := range cfgs {
		resolved, err := cfg.Resolve()
		if err != nil {
			return nil, err
		}

		url, err := resolved.GetURL()
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", resolved.Name, err)
		}

		username, err := resolved.GetUsername()
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", resolved.Name, err)
		}

		password, err := resolved.GetPassword()
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", resolved.Name, err)
		}

		var src calendar.Source

		switch resolved.Type {
		case "ics":
			src = calendar.NewICSSource(resolved.Name, url, username, password)

		case "caldav":
			src = calendar.NewCalDAVSource(resolved.Name, url, username, password, resolved.Calendars)

		case "icloud":
			src = calendar.NewICloudSource(resolved.Name, username, password, resolved.Calendars)

		case "ms365":
			src = calendar.NewMS365Source(resolved.Name)

		default:
			slog.Warn("unknown source type", "type", resolved.Type, "name", resolved.Name)
			continue
		}

		// Create per-source filter (if no rules, filter passes everything through)
		f, err := filter.New(resolved.Filters)
		if err != nil {
			return nil, err
		}

		sources = append(sources, sourceWithFilter{
			source: src,
			filter: f,
		})
	}

	return sources, nil
}
