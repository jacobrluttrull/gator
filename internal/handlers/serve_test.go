package handlers

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/config"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/testsupport"
)

func TestServeRejectsBadInterval(t *testing.T) {
	err := Serve(&cli.State{}, cli.Command{Name: "serve", Args: []string{"-interval", "nonsense"}})
	if err == nil {
		t.Fatal("Serve accepted -interval nonsense; want a flag parse error")
	}
}

// TestServeListenFailureStopsCleanly occupies the port first: Serve must
// return the listen error rather than hang, and must not leak the
// Aggregation goroutine while doing so.
func TestServeListenFailureStopsCleanly(t *testing.T) {
	db := testsupport.OpenTestDB(t)

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("closing occupied port: %v", err)
		}
	})
	port := ln.Addr().(*net.TCPAddr).Port

	s := &cli.State{Config: &config.Config{}, DB: database.New(db)}
	done := make(chan error, 1)
	go func() {
		done <- Serve(s, cli.Command{Name: "serve", Args: []string{
			"-port", strconv.Itoa(port), "-interval", "1h",
		}})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Serve returned nil on an occupied port; want the listen error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve didn't return after a listen failure; aggregation goroutine likely leaked")
	}
}
