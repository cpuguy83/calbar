package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cpuguy83/calbar/internal/calendar"
)

type testSource struct {
	name  string
	fetch func(context.Context, time.Time) ([]calendar.Event, error)
}

func (s testSource) Name() string { return s.name }

func (s testSource) Fetch(ctx context.Context, end time.Time) ([]calendar.Event, error) {
	return s.fetch(ctx, end)
}

func TestSyncReturnsPartialResultsWhenSourceTimesOut(t *testing.T) {
	origTimeout := sourceFetchTimeout
	sourceFetchTimeout = 25 * time.Millisecond
	t.Cleanup(func() { sourceFetchTimeout = origTimeout })

	fastEvent := calendar.Event{
		UID:     "fast",
		Summary: "Fast source event",
		Start:   time.Now().Add(time.Hour),
		End:     time.Now().Add(2 * time.Hour),
	}

	s := &Syncer{
		sources: []sourceWithFilter{
			{
				source: testSource{
					name: "fast",
					fetch: func(ctx context.Context, end time.Time) ([]calendar.Event, error) {
						return []calendar.Event{fastEvent}, nil
					},
				},
			},
			{
				source: testSource{
					name: "blocked",
					fetch: func(ctx context.Context, end time.Time) ([]calendar.Event, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
				},
			},
		},
		timeRange: 24 * time.Hour,
	}

	start := time.Now()
	events, failures, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error with partial results: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Sync took too long: %s", elapsed)
	}
	if len(events) != 1 || events[0].UID != fastEvent.UID {
		t.Fatalf("Sync events = %#v, want fast event", events)
	}
	if len(failures) != 1 || failures[0].Name != "blocked" {
		t.Fatalf("Sync failures = %#v, want blocked source failure", failures)
	}
	if !errors.Is(failures[0].Err, context.DeadlineExceeded) {
		t.Fatalf("failure err = %v, want deadline exceeded", failures[0].Err)
	}
}
