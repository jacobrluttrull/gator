# gator

A CLI RSS feed aggregator written in Go, backed by Postgres. Register users,
follow RSS feeds, and gator will periodically fetch new posts in the
background so you can browse them from the terminal.

## Requirements

Before running `gator`, make sure you have the following installed:

- [Go](https://go.dev/doc/install) (1.21+)
- [Postgres](https://www.postgresql.org/download/) — gator stores users, feeds, and posts in a Postgres database

## Installing the CLI

Install the `gator` binary directly from GitHub with `go install`:

```
go install github.com/jacobrluttrull/gator/cmd/gator@latest
```

This puts a `gator` executable in your `$GOPATH/bin` (or `$HOME/go/bin`), so make sure that directory is on your `PATH`.

## Setup

1. Create a Postgres database for gator, e.g.:
   ```
   createdb gator
   ```
2. Create a config file at `~/.gatorconfig.json` with your database connection string:
   ```json
   {
     "db_url": "postgres://user:password@localhost:5432/gator?sslmode=disable"
   }
   ```
3. Run the database migrations (requires [goose](https://github.com/pressly/goose)):
   ```
   goose -dir sql/schema postgres "postgres://user:password@localhost:5432/gator?sslmode=disable" up
   ```

## Running gator

Once installed and configured, run commands like:

```
gator register myusername
gator addFeed "Hacker News" "https://news.ycombinator.com/rss"
gator agg 1m
```

### A few commands to get you started

- `register <name>` — create a new user and log in as them
- `login <name>` — switch the current user
- `addFeed <name> <url>` — add a new RSS feed and automatically follow it
- `follow <url>` / `unfollow <url>` — follow or unfollow an existing feed
- `following` — list the feeds you're currently following
- `feeds` — list every feed that's been added, and who added it
- `agg <time_between_reqs>` — continuously fetch the least-recently-updated feed on a timer (e.g. `gator agg 1m`); leave this running in a terminal to keep collecting posts, stop it with `Ctrl+C`
- `browse [limit] [asc] [page]` — print posts from feeds you follow, newest first by default (e.g. `gator browse 5 asc 2` for page 2, oldest-first, 5 per page)
- `bookmark <post_url>` / `unbookmark <post_url>` — bookmark or remove a bookmark on a post
- `bookmarks` — list your bookmarked posts
- `search <term> [limit]` — fuzzy-search the titles of posts from feeds you follow (defaults to 5 results)
- `serve [-port 8080] [-interval 1m]` — runs the Service: the `/v1` HTTP API plus the aggregation loop as an in-process goroutine; `Ctrl+C` (or SIGTERM) stops both cleanly

## Project layout

```
cmd/gator/              entrypoint: config, DB setup, command registration, dispatch
internal/cli/           State, Command, Commands (registry + dispatch), the LoggedIn middleware
internal/handlers/      one file per command domain (auth, users, feeds, posts, agg)
internal/feed/          RSS fetching + XML parsing
internal/scraper/       pulls the next feed to fetch and saves new posts; the aggregation loop
internal/config/        reads/writes ~/.gatorconfig.json
internal/database/      sqlc-generated query code — do not hand-edit, edit sql/queries/ instead
sql/schema/             goose migrations
sql/queries/            sqlc query definitions
```
