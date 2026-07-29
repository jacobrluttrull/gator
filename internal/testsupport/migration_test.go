// This test lives in testsupport because it tests goose migrations rather
// than any one production package, and testsupport is what owns the test
// database's lifecycle. It reconstructs the pre-migration shape (no
// unique constraint, duplicate-URL feeds) and runs the migrations' real Up
// SQL against it, in version order.
package testsupport_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/testsupport"
)

// migrationUpSQL returns the Up half of the one migration matching glob,
// so these tests exercise the real files rather than a copy that can
// drift from them.
func migrationUpSQL(t *testing.T, glob string) string {
	t.Helper()
	matches, err := filepath.Glob("../../sql/schema/" + glob)
	if err != nil || len(matches) != 1 {
		t.Fatalf("locating migration %s: matches=%v err=%v", glob, matches, err)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[0], err)
	}
	up, _, found := strings.Cut(string(body), "-- +goose Down")
	if !found {
		t.Fatalf("%s has no -- +goose Down marker", matches[0])
	}
	return strings.TrimPrefix(strings.TrimSpace(up), "-- +goose Up")
}

// dedupThenConstraint returns the two migrations' Up SQL in the order
// goose runs them: the dedup is deliberately versioned ahead of the
// constraint, because goose stops at the first failing migration and the
// constraint is what fails on a database holding duplicates.
func dedupThenConstraint(t *testing.T) (dedup, constraint string) {
	t.Helper()
	dedup = migrationUpSQL(t, "*_dedup_feed_urls.sql")
	constraint = migrationUpSQL(t, "*_add_feed_url_unique.sql")

	dedupFile, _ := filepath.Glob("../../sql/schema/*_dedup_feed_urls.sql")
	constraintFile, _ := filepath.Glob("../../sql/schema/*_add_feed_url_unique.sql")
	if filepath.Base(dedupFile[0]) >= filepath.Base(constraintFile[0]) {
		t.Fatalf("dedup migration %s must sort before %s, or goose will abort on the constraint before it runs",
			filepath.Base(dedupFile[0]), filepath.Base(constraintFile[0]))
	}
	return dedup, constraint
}

func mustExec(t *testing.T, db *sql.DB, label, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

// feedURLKeyExists reports whether the unique constraint is on the table.
func feedURLKeyExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(context.Background(),
		`select exists(select 1 from pg_constraint where conname = 'feeds_url_key')`,
	).Scan(&exists); err != nil {
		t.Fatalf("checking for feeds_url_key: %v", err)
	}
	return exists
}

// TestConstraintMigrationAloneStillFailsOnDuplicates pins why the dedup
// is versioned ahead of the constraint rather than added as a follow-up:
// the constraint migration by itself aborts on a database holding
// duplicates, and goose stops at the first failing migration, so anything
// ordered after it would never run.
func TestConstraintMigrationAloneStillFailsOnDuplicates(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	dedup, constraint := dedupThenConstraint(t)

	mustExec(t, db, "dropping feeds_url_key", `alter table feeds drop constraint if exists feeds_url_key`)
	t.Cleanup(func() {
		if !feedURLKeyExists(t, db) {
			mustExec(t, db, "restoring feeds_url_key", dedup+constraint)
		}
	})

	mustExec(t, db, "seeding user", `
		insert into users (id, created_at, updated_at, name)
		values ('11111111-1111-1111-1111-111111111111', now(), now(), 'alice')`)
	mustExec(t, db, "seeding duplicate feeds", `
		insert into feeds (id, created_at, updated_at, name, url, user_id) values
			(gen_random_uuid(), '2026-01-01', '2026-01-01', 'one', 'https://dup.example/rss', '11111111-1111-1111-1111-111111111111'),
			(gen_random_uuid(), '2026-02-01', '2026-02-01', 'two', 'https://dup.example/rss', '11111111-1111-1111-1111-111111111111')`)

	if _, err := db.ExecContext(context.Background(), constraint); err == nil {
		t.Fatal("constraint migration succeeded on duplicate urls; the dedup migration's ordering no longer matters — re-check why it exists")
	}
}

// TestFeedURLUniqueMigrationCollapsesDuplicates covers the upgrade path
// for a database written before anything stopped two users adding the
// same feed URL: `goose up` must collapse the duplicates instead of
// aborting on "could not create unique index".
func TestFeedURLUniqueMigrationCollapsesDuplicates(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	dedup, constraint := dedupThenConstraint(t)

	// Roll the schema back to its pre-migration shape. If the migrations
	// under test fail, put the constraint back so later runs start clean.
	mustExec(t, db, "dropping feeds_url_key", `alter table feeds drop constraint if exists feeds_url_key`)
	t.Cleanup(func() {
		if !feedURLKeyExists(t, db) {
			mustExec(t, db, "restoring feeds_url_key", dedup+constraint)
		}
	})

	const dupURL = "https://shared.example/rss"
	const tripleURL = "https://triple.example/rss"
	mustExec(t, db, "seeding users", `
		insert into users (id, created_at, updated_at, name) values
			('11111111-1111-1111-1111-111111111111', now(), now(), 'alice'),
			('22222222-2222-2222-2222-222222222222', now(), now(), 'bob'),
			('33333333-3333-3333-3333-333333333333', now(), now(), 'carol'),
			('44444444-4444-4444-4444-444444444444', now(), now(), 'dave')`)
	// Two feeds on one URL (alice's is older, so it is the canonical row),
	// plus three feeds on another — the shape where follow collisions can
	// happen among the losing duplicates themselves, not just against the
	// canonical row.
	mustExec(t, db, "seeding duplicate feeds", `
		insert into feeds (id, created_at, updated_at, name, url, user_id) values
			('aaaaaaaa-0000-0000-0000-000000000001', '2026-01-01', '2026-01-01', 'Shared (alice)', $1, '11111111-1111-1111-1111-111111111111'),
			('bbbbbbbb-0000-0000-0000-000000000002', '2026-02-01', '2026-02-01', 'Shared (bob)',   $1, '22222222-2222-2222-2222-222222222222'),
			('cccccccc-0000-0000-0000-000000000003', '2026-03-01', '2026-03-01', 'Untouched', 'https://other.example/rss', '33333333-3333-3333-3333-333333333333'),
			('dddddddd-0000-0000-0000-000000000004', '2026-01-01', '2026-01-01', 'Triple (alice)', $2, '11111111-1111-1111-1111-111111111111'),
			('eeeeeeee-0000-0000-0000-000000000005', '2026-02-01', '2026-02-01', 'Triple (bob)',   $2, '22222222-2222-2222-2222-222222222222'),
			('ffffffff-0000-0000-0000-000000000006', '2026-03-01', '2026-03-01', 'Triple (carol)', $2, '33333333-3333-3333-3333-333333333333')`,
		dupURL, tripleURL)
	// Alice follows both duplicates — the case that collides on
	// unique(user_id, feed_id) when the follows are repointed. Bob follows
	// only the losing row; carol follows an unrelated feed.
	//
	// On the three-way URL: dave follows the two *losing* rows and not the
	// canonical one — the collision a canonical-only check misses — and
	// carol follows all three, colliding both against the canonical row
	// and between the losers.
	mustExec(t, db, "seeding follows", `
		insert into feed_follows (id, created_at, updated_at, user_id, feed_id) values
			('f0000000-0000-0000-0000-000000000001', now(), now(), '11111111-1111-1111-1111-111111111111', 'aaaaaaaa-0000-0000-0000-000000000001'),
			('f0000000-0000-0000-0000-000000000002', now(), now(), '11111111-1111-1111-1111-111111111111', 'bbbbbbbb-0000-0000-0000-000000000002'),
			('f0000000-0000-0000-0000-000000000003', now(), now(), '22222222-2222-2222-2222-222222222222', 'bbbbbbbb-0000-0000-0000-000000000002'),
			('f0000000-0000-0000-0000-000000000004', now(), now(), '33333333-3333-3333-3333-333333333333', 'cccccccc-0000-0000-0000-000000000003'),
			('f0000000-0000-0000-0000-000000000005', now(), now(), '44444444-4444-4444-4444-444444444444', 'eeeeeeee-0000-0000-0000-000000000005'),
			('f0000000-0000-0000-0000-000000000006', now(), now(), '44444444-4444-4444-4444-444444444444', 'ffffffff-0000-0000-0000-000000000006'),
			('f0000000-0000-0000-0000-000000000007', now(), now(), '33333333-3333-3333-3333-333333333333', 'dddddddd-0000-0000-0000-000000000004'),
			('f0000000-0000-0000-0000-000000000008', now(), now(), '33333333-3333-3333-3333-333333333333', 'eeeeeeee-0000-0000-0000-000000000005'),
			('f0000000-0000-0000-0000-000000000009', now(), now(), '33333333-3333-3333-3333-333333333333', 'ffffffff-0000-0000-0000-000000000006')`)
	mustExec(t, db, "seeding posts", `
		insert into posts (id, created_at, updated_at, title, url, description, published_at, feed_id) values
			('90000000-0000-0000-0000-000000000001', now(), now(), 'from alice''s copy', 'https://shared.example/1', '', now(), 'aaaaaaaa-0000-0000-0000-000000000001'),
			('90000000-0000-0000-0000-000000000002', now(), now(), 'from bob''s copy',   'https://shared.example/2', '', now(), 'bbbbbbbb-0000-0000-0000-000000000002'),
			('90000000-0000-0000-0000-000000000003', now(), now(), 'unrelated',          'https://other.example/1',  '', now(), 'cccccccc-0000-0000-0000-000000000003')`)

	// Both migrations, in the order goose runs them. They must survive this
	// database, not abort on it.
	if _, err := db.ExecContext(context.Background(), dedup); err != nil {
		t.Fatalf("dedup migration failed on a database with duplicate feed urls: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), constraint); err != nil {
		t.Fatalf("constraint migration failed after the dedup ran: %v", err)
	}

	var feedCount int
	if err := db.QueryRowContext(context.Background(), `select count(*) from feeds where url = $1`, dupURL).Scan(&feedCount); err != nil {
		t.Fatalf("counting feeds: %v", err)
	}
	if feedCount != 1 {
		t.Fatalf("feeds on the duplicated url = %d, want 1", feedCount)
	}

	var survivor, survivorName string
	if err := db.QueryRowContext(context.Background(), `select id::text, name from feeds where url = $1`, dupURL).Scan(&survivor, &survivorName); err != nil {
		t.Fatalf("reading surviving feed: %v", err)
	}
	if survivor != "aaaaaaaa-0000-0000-0000-000000000001" {
		t.Errorf("surviving feed = %s (%q), want the oldest row (alice's)", survivor, survivorName)
	}

	var tripleSurvivor string
	if err := db.QueryRowContext(context.Background(), `select id::text from feeds where url = $1`, tripleURL).Scan(&tripleSurvivor); err != nil {
		t.Fatalf("reading the three-way url's surviving feed: %v", err)
	}
	if tripleSurvivor != "dddddddd-0000-0000-0000-000000000004" {
		t.Errorf("three-way url's surviving feed = %s, want the oldest row (alice's)", tripleSurvivor)
	}

	// Nobody loses their Following, and every collision is collapsed rather
	// than duplicated: alice followed both copies of the shared feed, dave
	// followed two losing copies of the triple feed, carol followed all
	// three — each ends with exactly one.
	for _, tc := range []struct{ user, url string }{
		{"alice", dupURL},
		{"bob", dupURL},
		{"dave", tripleURL},
		{"carol", tripleURL},
	} {
		var follows int
		if err := db.QueryRowContext(context.Background(), `
			select count(*) from feed_follows ff
			join users u on u.id = ff.user_id
			join feeds f on f.id = ff.feed_id
			where u.name = $1 and f.url = $2`, tc.user, tc.url).Scan(&follows); err != nil {
			t.Fatalf("counting %s's follows: %v", tc.user, err)
		}
		if follows != 1 {
			t.Errorf("%s's followings of %s = %d, want 1", tc.user, tc.url, follows)
		}
	}

	// Posts from both copies survive, repointed onto the canonical feed.
	var repointed int
	if err := db.QueryRowContext(context.Background(), `select count(*) from posts where feed_id = $1`, survivor).Scan(&repointed); err != nil {
		t.Fatalf("counting repointed posts: %v", err)
	}
	if repointed != 2 {
		t.Errorf("posts on the surviving feed = %d, want both copies' 2", repointed)
	}

	// The unrelated feed, its follow, and its post are untouched.
	var untouched int
	if err := db.QueryRowContext(context.Background(), `
		select count(*) from feeds f
		join feed_follows ff on ff.feed_id = f.id
		join posts p on p.feed_id = f.id
		where f.url = 'https://other.example/rss'`).Scan(&untouched); err != nil {
		t.Fatalf("checking the unrelated feed: %v", err)
	}
	if untouched != 1 {
		t.Errorf("unrelated feed's follow/post rows = %d, want 1 intact set", untouched)
	}

	// And the constraint the migration exists for is actually on.
	if !feedURLKeyExists(t, db) {
		t.Fatal("migration finished without adding feeds_url_key")
	}
	if _, err := db.ExecContext(context.Background(), `
		insert into feeds (id, created_at, updated_at, name, url, user_id)
		values (gen_random_uuid(), now(), now(), 'another', $1, '33333333-3333-3333-3333-333333333333')`,
		dupURL); err == nil {
		t.Error("inserting a duplicate url succeeded after the migration; want a unique violation")
	}
}
