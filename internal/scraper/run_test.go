package scraper

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// startRun runs Run in a goroutine, counting calls to scrape, and
// returns the call counter plus a channel closed when Run returns.
func startRun(ctx context.Context, interval time.Duration, scrape func() error) (*atomic.Int64, chan struct{}) {
	var calls atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, interval, func() error {
			calls.Add(1)
			return scrape()
		})
	}()
	return &calls, done
}

func waitFor(t *testing.T, label string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(label)
}

func TestRunScrapesImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// An hour-long interval: any call at all must be the immediate one.
	calls, done := startRun(ctx, time.Hour, func() error { return nil })
	waitFor(t, "Run never scraped; want an immediate scrape before the first tick", func() bool {
		return calls.Load() == 1
	})

	cancel()
	<-done
}

func TestRunContinuesAfterScrapeFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls, done := startRun(ctx, time.Millisecond, func() error { return errors.New("feed exploded") })
	waitFor(t, "Run stopped after scrape failures; want it to log and keep going", func() bool {
		return calls.Load() >= 3
	})

	cancel()
	<-done
}

func TestRunContinuesAfterScrapePanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ADR-0002: with no supervisor to respawn the process, the
	// Aggregation goroutine recovers and logs internally.
	calls, done := startRun(ctx, time.Millisecond, func() error { panic("feed exploded harder") })
	waitFor(t, "Run stopped after a scrape panic; want it recovered and logged", func() bool {
		return calls.Load() >= 3
	})

	cancel()
	<-done
}

func TestRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls, done := startRun(ctx, time.Millisecond, func() error { return nil })
	waitFor(t, "Run never scraped", func() bool { return calls.Load() >= 1 })

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run didn't return after context cancellation")
	}

	settled := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != settled {
		t.Errorf("scrapes continued after Run returned: %d -> %d", settled, got)
	}
}
