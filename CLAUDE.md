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
- `internal/handlers/` — command implementations, one file per domain: `auth.go` (login/register),
  `users.go` (reset/users), `feeds.go` (addFeed/feeds/follow/unfollow/following), `posts.go`
  (browse/bookmark/unbookmark/bookmarks/search), `agg.go`
- `internal/feed/` — `Fetch`: HTTP fetch + XML parsing of an RSS feed into `RSSFeed`/`RSSItem`
- `internal/scraper/` — `Scrape`: pulls the next feed to fetch, marks it fetched, saves new posts;
  `ParsePublishedAt` for RSS date parsing
- `internal/supervisor/` — `Serve`: runs `agg` as a supervised child process with crash-restart + backoff
- `internal/config/` — reads/writes `~/.gatorconfig.json` (`db_url`, `current_user_name`)
- `internal/database/` — sqlc-generated code. **Never hand-edit files here** — edit the `.sql` in `sql/queries/` and run `sqlc generate`
- `sql/schema/` — goose migrations, one file per schema change, timestamp-prefixed
- `sql/queries/` — one query per file, named to match the query (e.g. `getfeedbyurl.sql` → `GetFeedByUrl`)

Package dependency direction: `cli` has no dependency on the others; `feed` is standalone;
`scraper` depends on `cli` + `feed` + `database`; `handlers` depends on `cli` + `scraper` +
`database`; `supervisor` depends on `cli` only (it shells out to the `gator` binary itself for
`agg`, it doesn't call handlers directly). `cmd/gator` wires all of them together.

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
  already-created/applied migration. Use `goose -dir sql/schema create <name> sql`.
- Queries that insert a row but need to return joined data from another table (e.g.
  `CreateFeedFollow` returning the user's and feed's names) use a `WITH ... AS (INSERT ... RETURNING *)`
  CTE, then `SELECT` + `JOIN` off that CTE — sqlc can't return cross-table columns from a bare `INSERT ... RETURNING`.
- Timestamps are passed in from Go (`time.Now().UTC()`), not generated with SQL `NOW()`.
- Fuzzy search on post titles uses `pg_trgm`'s `%` operator + `similarity()`, both sides wrapped
  in `lower()` for case-insensitivity. The GIN trigram index is on plain `title`, not `lower(title)`,
  so it isn't actually used by the current query — acceptable at this data size, would need a
  functional index rebuild if that ever matters.

## Running migrations / regenerating code

```
goose -dir sql/schema postgres "$DB_URL" up      # apply pending migrations
sqlc generate                                     # regenerate internal/database/ after editing sql/queries/
go build ./...                                    # confirm it compiles after either
```

## Gotchas hit before

- If a sqlc query reuses the same `$n` placeholder for two columns (e.g. `updated_at = $2,
  last_fetched_at = $2`), sqlc collapses them into a **single** generated param — don't expect
  two separate struct fields.
- If a query wraps a param in a SQL function (e.g. `lower($2)`), sqlc names the generated
  struct field after the function (`Lower`), not the semantic meaning (`Similarity`) — check
  the generated `.sql.go` file rather than assuming the field name.
- `agg` runs forever via `time.Ticker` in a `for ; ; <-ticker.C` loop — stop it with `Ctrl+C`.
  `serve` supervises it as a child process instead and restarts it on crash with backoff.
- New Postgres extensions/indexes (like `pg_trgm`) are schema migrations (DDL), not queries.
- Go test files must live in the same directory as the package they test — a top-level `tests/`
  directory can't reach unexported identifiers (like `supervisor.nextBackoff`) and doesn't work
  the way it might in other languages. Tests stay next to their package.
- `go install` now builds from `./cmd/gator`, not the repo root — `go.mod`'s module path
  (`github.com/jacobrluttrull/gator`) plus the `cmd/gator` subpath is what `go install
  github.com/jacobrluttrull/gator/cmd/gator@latest` resolves.
