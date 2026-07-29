package handlers

import (
	"context"
	"net"
	"strconv"
	"strings"
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

// TestServeRejectsPositionalArgs covers the old `gator serve 1m` form the
// deleted supervisor.Serve accepted. Flag parsing stops at the first
// non-flag argument, so these used to run at the default interval with
// any following flags silently dropped — the failure has to be loud.
func TestServeRejectsPositionalArgs(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"bare old-style interval", []string{"30s"}},
		{"old-style interval before flags", []string{"30s", "-port", "9090"}},
		{"stray trailing argument", []string{"-port", "9090", "extra"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Serve(&cli.State{}, cli.Command{Name: "serve", Args: tt.args})
			if err == nil {
				t.Fatalf("Serve(%q) = nil; want an error rather than a silent default interval", tt.args)
			}
			// The message has to point at the replacement, not just complain.
			if !strings.Contains(err.Error(), "-interval") {
				t.Errorf("Serve(%q) error = %q, want it to name -interval", tt.args, err)
			}
		})
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
