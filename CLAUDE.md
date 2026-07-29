# gator

CLI RSS feed aggregator in Go, backed by Postgres. Users register, add/follow
RSS feeds, and a background aggregator (`agg`) periodically fetches new posts,
which can then be browsed, searched, and bookmarked from the terminal.

## Stack

- Go (module `github.com/jacobrluttrull/gator`), `database/sql` + `lib/pq`
- Postgres, with `pg_trgm` enabled for fuzzy search
- [goose](https://github.com/pressly/goose) for schema migrations (`sql/schema/`)
- [sqlc](https://sqlc.dev/) to generate typed Go query code (`internal/database/`) from `sql/queries/`

## File layout

- `cmd/gator/main.go` — entrypoint: loads config, opens DB, builds the command map, dispatches
- `internal/cli/` — `State`, `Command`, `Commands` (registry + `Run`), and the `LoggedIn` middleware
- `internal/handlers/` — command implementations, one file per domain: `auth.go` (login/register/setpassword),
  `users.go` (reset/users), `feeds.go` (addFeed/feeds/follow/unfollow/following), `posts.go`
  (browse/bookmark/unbookmark/bookmarks/search), `agg.go`, `serve.go` (runs the Service: mounts the
  `/v1` API on a stdlib HTTP server plus the Aggregation loop as an in-process goroutine per
  ADR-0002, `-port` and `-interval` flags)
- `internal/api/` — `New(db)` returns the `/v1` `http.Handler` (REST API per ADR-0001/0002);
  handlers respond with sqlc-row-mirror JSON (except login, which returns the one-time
  `{"api_key": ...}` — the row only holds the hash; add-feed nests two mirrors as
  `{"feed": ..., "feed_follow": ...}`), errors as `{"error": "<message>"}`.
  Authenticated routes wrap an `authedHandler` with the `loggedIn` middleware (the HTTP
  analogue of `cli.LoggedIn`): it resolves `Authorization: ApiKey <key>` to a user and
  fails closed with a uniform 401. One file per domain, mirroring `internal/handlers/`:
  `register.go`, `login.go`, `feeds.go`, `follows.go`, `posts.go`, `bookmarks.go`
  (`GET`/`POST`/`DELETE /v1/bookmarks`), `search.go` (`GET /v1/search?q=&limit=`).
  Endpoints that identify a row by URL take it in a `{"url": ...}` body (`urlFromBody`);
  `/v1/posts` and `/v1/search` take an optional `limit` query param (`limitParam`) —
  both shared helpers live in `api.go` and write their own 4xx responses. Routes are a
  table in `New` rather than a run of `mux.HandleFunc` calls, because the `"/"` catch-all
  (`handleUnmatched`) reuses it to answer unmatched paths with a JSON 404 and wrong verbs
  with a JSON 405 + `Allow` header — registering `"/"` takes over the mux's own plain-text
  405, so the route table is what keeps `Allow` honest
- `internal/auth/` — pure helpers: bcrypt password hash/verify, API key generate/SHA-256 hash
- `internal/feed/` — `Fetch`: HTTP fetch + XML parsing of an RSS feed into `RSSFeed`/`RSSItem`
- `internal/scraper/` — `Scrape`: pulls the next feed to fetch, marks it fetched, saves new posts;
  `Run`: the Aggregation loop (scrape immediately, then every interval, log-and-skip failures,
  stop on context cancel) shared by `agg` and `serve`; `ParsePublishedAt` for RSS date parsing
- `internal/config/` — reads/writes `~/.gatorconfig.json` (`db_url`, `current_user_name`)
- `internal/testsupport/` — test-only harness: `OpenTestDB` (open + advisory-lock + truncate
  the `GATOR_TEST_DB_URL` database), shared by the `api` and `handlers` integration tests
- `internal/database/` — sqlc-generated code. **Never hand-edit files here** — edit the `.sql` in `sql/queries/` and run `sqlc generate`
- `sql/schema/` — goose migrations, one file per schema change, timestamp-prefixed
- `sql/queries/` — one query per file, named to match the query (e.g. `getfeedbyurl.sql` → `GetFeedByUrl`)

Package dependency direction: `cli` has no dependency on the others; `feed` and `auth` are
standalone; `api` depends on `auth` + `database`; `scraper` depends on `cli` + `feed` +
`database`; `handlers` depends on `cli` + `scraper` + `api` + `database`. `cmd/gator` wires
all of them together.

## Command pattern

Every command handler has one of two signatures:

```go
func X(s *cli.State, cmd cli.Command) error                          // no login required
func X(s *cli.State, cmd cli.Command, user database.User) error      // requires the logged-in user
```

The second form is wrapped with `cli.LoggedIn(X)` at registration, which looks up the current
user (via `s.Config.CurrentUserName`) and injects it — handlers needing "who's logged in" never
call `GetUser` themselves. Register new commands in the `cmds.Handlers` map literal in
`cmd/gator/main.go`, not via a loop or repeated calls.

## Database conventions

- Every table has `id UUID primary key`, `created_at`/`updated_at timestamp not null`.
- Single-column constraints (`unique`, `not null`, `references ... on delete cascade`) go inline
  on the column. Multi-column constraints (composite `unique(a, b)`) go as a trailing table-level line.
- Foreign keys use `on delete cascade` — deleting a user cascades to their feeds, follows, posts, bookmarks.
- **New tables or schema changes always get a new goose migration file** — never edit an
  already-created/applied migration, even one only applied locally on an unmerged branch.
  Use `goose -dir sql/schema create <name> sql`.
- A constraint that existing rows can violate needs its data cleanup in a **separate migration
  versioned _before_ the constraint** — goose applies in version order and stops at the first
  failure, so a cleanup ordered after would never run (`20260729041443_dedup_feed_urls.sql`
  ahead of `20260729041444_add_feed_url_unique.sql`). That means hand-naming the version
  instead of `goose create`, and a database that already applied the later migration needs a
  one-time `goose ... -allow-missing up` (plain `up` refuses with "found 1 missing migrations").
  `internal/testsupport/migration_test.go` guards both the ordering and the cleanup.
- Queries that insert a row but need to return joined data from another table (e.g.
  `CreateFeedFollow` returning the user's and feed's names) use a `WITH ... AS (INSERT ... RETURNING *)`
  CTE, then `SELECT` + `JOIN` off that CTE — sqlc can't return cross-table columns from a bare `INSERT ... RETURNING`.
- Timestamps are passed in from Go (`time.Now().UTC()`), not generated with SQL `NOW()`.
- Fuzzy search on post titles uses `pg_trgm`'s `%` operator + `similarity()`, both sides wrapped
  in `lower()` for case-insensitivity. The GIN trigram index is on plain `title`, not `lower(title)`,
  so it isn't actually used by the current query — acceptable at this data size, would need a
  functional index rebuild if that ever matters.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (jacobrluttrull/gator), via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout (`CONTEXT.md` + `docs/adr/` at the repo root). See `docs/agents/domain.md`.

## Running migrations / regenerating code

```
goose -dir sql/schema postgres "$DB_URL" up      # apply pending migrations
sqlc generate                                     # regenerate internal/database/ after editing sql/queries/
go build ./...                                    # confirm it compiles after either
```

## Running tests

Integration tests (`internal/api`, `internal/handlers`) need `GATOR_TEST_DB_URL` set to a
**dedicated test database** with migrations applied (they skip when unset). The shared
harness (`internal/testsupport.OpenTestDB`) `TRUNCATE`s tables between tests — never point
it at a database whose data you care about — and serializes DB-touching tests across
packages with a Postgres advisory lock, since `go test ./...` runs packages in parallel.

```
GATOR_TEST_DB_URL="postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" go test ./...
```

## Gotchas hit before

- If a sqlc query reuses the same `$n` placeholder for two columns (e.g. `updated_at = $2,
  last_fetched_at = $2`), sqlc collapses them into a **single** generated param — don't expect
  two separate struct fields.
- If a query wraps a param in a SQL function (e.g. `lower($2)`), sqlc names the generated
  struct field after the function (`Lower`), not the semantic meaning (`Similarity`) — check
  the generated `.sql.go` file rather than assuming the field name.
- `agg` runs the Aggregation loop forever in the foreground — stop it with `Ctrl+C`. `serve`
  runs the same loop as an in-process goroutine and stops it via context on SIGINT/SIGTERM;
  process-level restarts belong to the deployment environment (ADR-0002), not the app.
- New Postgres extensions/indexes (like `pg_trgm`) are schema migrations (DDL), not queries.
- Go test files must live in the same directory as the package they test — a top-level `tests/`
  directory can't reach unexported identifiers and doesn't work
  the way it might in other languages. Tests stay next to their package.
- `go install` now builds from `./cmd/gator`, not the repo root — `go.mod`'s module path
  (`github.com/jacobrluttrull/gator`) plus the `cmd/gator` subpath is what `go install
  github.com/jacobrluttrull/gator/cmd/gator@latest` resolves.
