package scraper

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Run is the Aggregation loop: it calls scrape immediately and then once
// per interval until ctx is cancelled. A failed scrape (bad feed, network
// error) is logged and skipped, never fatal to the loop — with no
// supervisor to respawn the process (ADR-0002), even a panic is recovered.
func Run(ctx context.Context, interval time.Duration, scrape func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := safeScrape(scrape); err != nil {
			log.Printf("scrape failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func safeScrape(scrape func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scrape panicked: %v", r)
		}
	}()
	return scrape()
}
