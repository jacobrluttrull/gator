package handlers

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jacobrluttrull/gator/internal/api"
	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/scraper"
)

// Serve runs the Service: the /v1 API on a stdlib HTTP server plus the
// Aggregation loop as an in-process goroutine (ADR-0002), shutting both
// down cleanly on SIGINT/SIGTERM.
func Serve(s *cli.State, cmd cli.Command) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8080, "port for the HTTP API")
	interval := fs.Duration("interval", time.Minute, "time between feed fetches")
	if err := fs.Parse(cmd.Args); err != nil {
		return err
	}
	// The deleted supervisor.Serve took a positional interval ("serve 1m").
	// Flag parsing stops at the first non-flag argument, so an old-style
	// invocation would otherwise run at the default interval — and quietly
	// drop every flag after it, including -port.
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q: serve takes flags only — use -interval (e.g. `serve -interval 1m -port 8080`)", fs.Arg(0))
	}
	// scraper.Run rejects this too, but from inside the goroutine — check
	// here so a bad flag fails the command before anything starts.
	if *interval <= 0 {
		return fmt.Errorf("-interval must be positive, got %s", *interval)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	aggDone := make(chan struct{})
	go func() {
		defer close(aggDone)
		if err := scraper.Run(ctx, *interval, func(ctx context.Context) error {
			return scraper.Scrape(ctx, s)
		}); err != nil {
			log.Printf("aggregation loop: %v", err)
		}
	}()
	log.Printf("aggregating feeds every %s", *interval)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: api.New(s.DB, s.Conn),
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ListenAndServe()
	}()
	log.Printf("serving API on :%d", *port)

	select {
	case err := <-serveErr:
		stop()
		<-aggDone
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	<-aggDone
	if err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
