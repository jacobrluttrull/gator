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

- `main.go` — entrypoint: loads config, opens DB, builds the command map, dispatches
- `commands.go` — `state`, `command`, `commands` types, `run`, and `middlewareLoggedIn`
- `handlers.go` — every `handlerX` command implementation
- `fetchFeed.go` — HTTP fetch + XML parsing of an RSS feed into `RSSFeed`/`RSSItem`
- `scrapeFeeds.go` — `scrapFeeds`: pulls the next feed to fetch, marks it fetched, saves new posts
- `internal/config/` — reads/writes `~/.gatorconfig.json` (`db_url`, `current_user_name`)
- `internal/database/` — sqlc-generated code. **Never hand-edit files here** — edit the `.sql` in `sql/queries/` and run `sqlc generate`
- `sql/schema/` — goose migrations, one file per schema change, timestamp-prefixed
- `sql/queries/` — one query per file, named to match the query (e.g. `getfeedbyurl.sql` → `GetFeedByUrl`)

## Command pattern

Every command handler has one of two signatures:

```go
func handlerX(s *state, cmd command) error                          // no login required
func handlerX(s *state, cmd command, user database.User) error      // requires the logged-in user
```

The second form is wrapped with `middlewareLoggedIn(handlerX)` at registration, which looks up
the current user (via `s.Config.CurrentUserName`) and injects it — handlers needing "who's
logged in" never call `GetUser` themselves. Register new commands in the `cmds.commands` map
literal in `main.go`, not via a loop or repeated calls.

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
- New Postgres extensions/indexes (like `pg_trgm`) are schema migrations (DDL), not queries.
