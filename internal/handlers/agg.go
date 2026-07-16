package handlers

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/scraper"
)

func Agg(s *cli.State, cmd cli.Command) error {
	if len(cmd.Args) != 1 {
		return errors.New("the agg handler expects a single argument: time_between_reqs")
	}
	timeBetweenRequests, err := time.ParseDuration(cmd.Args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Collecting feeds every %s\n", timeBetweenRequests)

	ticker := time.NewTicker(timeBetweenRequests)
	for ; ; <-ticker.C {
		if err := scraper.Scrape(s); err != nil {
			log.Printf("scrape failed: %v", err)
		}
	}
}
