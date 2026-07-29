# gator

An RSS feed aggregator written in Go, backed by Postgres. Users follow feeds, a
background loop fetches new posts, and you browse, search, and bookmark them —
either from the terminal (CLI) or over HTTP (REST API).

There are two ways in, over the same database:

- **CLI** — the trusted local surface. It talks straight to Postgres. Having the
  DB URL *is* the credential, so there's no password on this side.
- **API** — the network surface. Every request needs an API key, and API keys
  come from a password login.

See [GUIDE.md](GUIDE.md) for the full walkthrough: how a command and a request
flow end to end, the data model, the aggregation loop, the complete API
reference, and a debugging playbook.

## Requirements

- [Go](https://go.dev/doc/install) 1.26+ (see `go.mod`)
- [Postgres](https://www.postgresql.org/download/) — stores users, feeds, posts, bookmarks, API keys
- [goose](https://github.com/pressly/goose) — runs the schema migrations
- [sqlc](https://sqlc.dev/) — only if you're editing queries

## Install

```
go install github.com/jacobrluttrull/gator/cmd/gator@latest
```

That drops a `gator` binary in `$GOPATH/bin` (usually `$HOME/go/bin`) — make
sure that's on your `PATH`. Note the `/cmd/gator` suffix: the binary is not at
the module root.

## Setup

1. Create the database:
   ```
   createdb gator
   ```
2. Create `~/.gatorconfig.json` (in your **home directory** — the config is read
   from `$HOME` and nowhere else; a copy in the repo root is ignored by git and
   never loaded):
   ```json
   {
     "db_url": "postgres://user:password@localhost:5432/gator?sslmode=disable"
   }
   ```
   `current_user_name` gets written in for you by `register` / `login`.
3. Apply migrations:
   ```
   goose -dir sql/schema postgres "postgres://user:password@localhost:5432/gator?sslmode=disable" up
   ```

## Quick start

```bash
gator register alice                                        # create a user, become them
gator addFeed "Hacker News" "https://news.ycombinator.com/rss"
gator agg 1m                                                # fetch loop, foreground — Ctrl+C to stop
# ...in another terminal:
gator browse 10                                             # newest 10 posts from feeds you follow
gator search postgres                                       # fuzzy title search
```

To run the API instead of a bare fetch loop:

```bash
gator setpassword hunter2      # a user needs a password before they can log in over HTTP
gator serve -port 8080 -interval 1m
curl -s localhost:8080/v1/login -d '{"name":"alice","password":"hunter2"}'
# => {"api_key":"..."}
curl -s localhost:8080/v1/posts -H "Authorization: ApiKey <key>"
```

## CLI commands

| Command | What it does |
| --- | --- |
| `register <name>` | Create a user and make them the current CLI user |
| `login <name>` | Switch the current CLI user (no password — see [ADR-0001](docs/adr/0001-two-surface-trust-model.md)) |
| `setpassword <password>` | Set the current user's password so they can log in over the API. Revokes their existing API keys |
| `users` | List users, marking the current one |
| `reset` | **Deletes every user** — and by cascade every feed, follow, post, and bookmark |
| `addFeed <name> <url>` | Add a feed to the shared pool and follow it |
| `feeds` | List every feed in the pool and who added it |
| `follow <url>` / `unfollow <url>` | Follow or unfollow a feed that already exists |
| `following` | List the feeds you follow |
| `browse [limit] [asc] [page]` | Print posts from feeds you follow. Defaults: limit 2, newest-first, page 1. e.g. `browse 5 asc 2` |
| `bookmark <post_url>` / `unbookmark <post_url>` | Bookmark a post, or remove the bookmark |
| `bookmarks` | List your bookmarked posts, newest bookmark first |
| `search <term> [limit]` | Fuzzy-search post titles across feeds you follow (default 5 results) |
| `agg <interval>` | Run the aggregation loop in the foreground for debugging, e.g. `agg 1m`. Ctrl+C stops it |
| `serve [-port 8080] [-interval 1m]` | Run the Service: the `/v1` API plus the aggregation loop in one process |

## API endpoints

All under `/v1`. Everything except `register` and `login` needs
`Authorization: ApiKey <key>`. Errors are always `{"error": "..."}`.

| Method | Path | Body / query | Returns |
| --- | --- | --- | --- |
| POST | `/v1/register` | `{"name","password"}` | 201 user |
| POST | `/v1/login` | `{"name","password"}` | 200 `{"api_key"}` |
| GET | `/v1/feeds` | — | 200 every feed in the pool |
| POST | `/v1/feeds` | `{"name","url"}` | 201 `{"feed","feed_follow"}` |
| GET | `/v1/follows` | — | 200 your follows |
| POST | `/v1/follows` | `{"url"}` | 201 the follow |
| DELETE | `/v1/follows` | `{"url"}` | 204 |
| GET | `/v1/posts` | `?limit=` (default 2) | 200 posts from feeds you follow |
| GET | `/v1/bookmarks` | — | 200 your bookmarked posts |
| POST | `/v1/bookmarks` | `{"url"}` | 201 the bookmark |
| DELETE | `/v1/bookmarks` | `{"url"}` | 204 |
| GET | `/v1/search` | `?q=` (required) `&limit=` (default 5) | 200 posts + `sim` score |
| DELETE | `/v1/keys` | — | 204 — revokes **all** your API keys |

Admin verbs (`reset`, `users`) are CLI-only on purpose. Full request/response
shapes and `curl` examples are in [GUIDE.md](GUIDE.md#api-reference).

## Project layout

```
cmd/gator/              entrypoint: read config, open DB, build command map, dispatch
internal/cli/           State, Command, Commands (registry + Run), the LoggedIn middleware
internal/handlers/      CLI command implementations, one file per domain
internal/api/           the /v1 HTTP handler, one file per domain, + the loggedIn middleware
internal/auth/          bcrypt password hashing, API key generation + SHA-256 hashing
internal/feed/          HTTP fetch + XML parse of an RSS feed
internal/scraper/       Scrape (one feed) and Run (the aggregation loop)
internal/config/        read/write ~/.gatorconfig.json
internal/testsupport/   shared integration-test harness
internal/database/      sqlc-generated — never hand-edit; edit sql/queries/ and regenerate
sql/schema/             goose migrations
sql/queries/            sqlc query definitions, one query per file
```

## Development

```bash
goose -dir sql/schema postgres "$DB_URL" up   # apply migrations
sqlc generate                                  # regenerate internal/database/ after editing sql/queries/
go build ./...                                 # confirm it still compiles

# integration tests need a dedicated, migrated test database — the harness
# TRUNCATEs it between tests, so never point this at real data
GATOR_TEST_DB_URL="postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" go test ./...
```

## Poking at the API

`api-tests/` has the whole user flow — register, login, add feed, follow,
browse, bookmark, search, revoke — with the API key captured from the login
response automatically. Edit the name and password at the top and run it.

```bash
gator serve -port 8080 -interval 1m   # in one terminal
./api-tests/run.sh                     # in another
```

- `api-tests/run.sh` — curl + bash, needs nothing installed. Prints a pass/fail
  table and exits non-zero on failure, so it also works as a smoke test
- `api-tests/gator.jetbrains.http` — GoLand / IntelliJ's built-in HTTP client
- `api-tests/gator.http` — VS Code REST Client syntax (also httpYac, Postman, Insomnia)

Zed note: the `http` extension in Zed's registry only *highlights* `.http`
files — it can't send requests. Use `run.sh` from Zed's terminal.

## Further reading

- [GUIDE.md](GUIDE.md) — the deep guide: architecture, data model, API reference, debugging
- [CONTEXT.md](CONTEXT.md) — the vocabulary (Feed vs Following, API key vs API login, ...)
- [docs/adr/](docs/adr/) — why the two-surface trust model and the single-process Service
- [EXTENDING.md](EXTENDING.md) — ideas not built yet
- [CLAUDE.md](CLAUDE.md) — conventions for agents working in this repo
