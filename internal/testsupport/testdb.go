// Package testsupport holds shared harness helpers for integration tests
// that run against the GATOR_TEST_DB_URL database.
package testsupport

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// testDBLock is the advisory-lock key serializing test packages on the
// shared test database. `go test ./...` runs packages in parallel, and
// every harness TRUNCATEs the same database — without the lock, one
// package's wipe races another's seeded rows.
const testDBLock = 20260728

// OpenTestDB opens the test database (skipping the test if
// GATOR_TEST_DB_URL is unset), takes the cross-package advisory lock for
// the duration of the test, and wipes the data to a clean slate.
// GATOR_TEST_DB_URL must point at a dedicated, migrated test database —
// every table reachable from users is truncated.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbURL := os.Getenv("GATOR_TEST_DB_URL")
	if dbURL == "" {
		t.Skip("GATOR_TEST_DB_URL not set; skipping integration test")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// The lock must live on one pinned connection: pg_advisory_lock is
	// session-scoped, and the pool would otherwise unlock on a different
	// session than the one that locked.
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("acquiring test-lock connection: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", testDBLock); err != nil {
		t.Fatalf("locking test database: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", testDBLock)
		conn.Close()
	})

	// Bounded so a stale lock-holder elsewhere can't hang the whole test
	// process; the advisory-lock wait above is deliberately unbounded
	// (queueing behind another package's tests is normal).
	truncateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.ExecContext(truncateCtx, "TRUNCATE users CASCADE"); err != nil {
		t.Fatalf("truncating test database: %v", err)
	}
	return db
}
