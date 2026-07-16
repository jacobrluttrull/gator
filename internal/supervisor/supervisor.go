package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/jacobrluttrull/gator/internal/cli"
)

const (
	initialBackoff = time.Second
	maxBackoff     = time.Minute
)

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

// Serve runs `agg` as a supervised child process, restarting it with
// exponential backoff if it exits unexpectedly. A manual Ctrl+C/SIGTERM
// stops it cleanly without restarting.
func Serve(s *cli.State, cmd cli.Command) error {
	if len(cmd.Args) != 1 {
		return errors.New("the serve handler expects a single argument: time_between_reqs")
	}
	timeBetweenRequests := cmd.Args[0]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	backoff := initialBackoff

	for ctx.Err() == nil {
		child := exec.CommandContext(ctx, os.Args[0], "agg", timeBetweenRequests)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr

		fmt.Println("serve: starting agg")
		err := child.Run()

		if ctx.Err() != nil {
			fmt.Println("serve: shutting down")
			return nil
		}

		fmt.Printf("serve: agg exited (%v), restarting in %s\n", err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}

		backoff = nextBackoff(backoff)
	}
	return nil
}
