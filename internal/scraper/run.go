package scraper

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Run is the Aggregation loop: it calls scrape immediately and then once
// per interval until ctx is cancelled. The scrape callback receives ctx so
// an in-flight fetch can stop on shutdown instead of blocking it. A failed
// scrape (bad feed, network error) is logged and skipped, never fatal to
// the loop — with no supervisor to respawn the process (ADR-0002), even a
// panic is recovered. A non-positive interval is an error, not a panic.
func Run(ctx context.Context, interval time.Duration, scrape func(context.Context) error) error {
	if interval <= 0 {
		return fmt.Errorf("aggregation interval must be positive, got %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := safeScrape(ctx, scrape); err != nil {
			log.Printf("scrape failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func safeScrape(ctx context.Context, scrape func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scrape panicked: %v", r)
		}
	}()
	return scrape(ctx)
}
