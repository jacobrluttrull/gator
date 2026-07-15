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
go install github.com/jacobrluttrull/gator@latest
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
gator addfeed "Hacker News" "https://news.ycombinator.com/rss"
gator agg 1m
```

### A few commands to get you started

- `register <name>` — create a new user and log in as them
- `login <name>` — switch the current user
- `addfeed <name> <url>` — add a new RSS feed and automatically follow it
- `follow <url>` / `unfollow <url>` — follow or unfollow an existing feed
- `following` — list the feeds you're currently following
- `feeds` — list every feed that's been added, and who added it
- `agg <time_between_reqs>` — continuously fetch the least-recently-updated feed on a timer (e.g. `gator agg 1m`); leave this running in a terminal to keep collecting posts, stop it with `Ctrl+C`
- `browse [limit]` — print the most recent posts from feeds you follow (defaults to 2)
