package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/scraper"
)

// Agg runs the Aggregation loop as a manual foreground debug loop; the
// deployed form is the in-process goroutine inside serve.
func Agg(s *cli.State, cmd cli.Command) error {
	if len(cmd.Args) != 1 {
		return errors.New("the agg handler expects a single argument: time_between_reqs")
	}
	timeBetweenRequests, err := time.ParseDuration(cmd.Args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Collecting feeds every %s\n", timeBetweenRequests)

	return scraper.Run(context.Background(), timeBetweenRequests, func(ctx context.Context) error {
		return scraper.Scrape(ctx, s)
	})
}
