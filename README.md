# gator

A CLI RSS feed aggregator written in Go, backed by Postgres.

## Requirements

- Go
- Postgres
- [goose](https://github.com/pressly/goose) (for migrations)
- [sqlc](https://sqlc.dev/) (for generating DB query code)

## Setup

1. Create a Postgres database.
2. Create a `~/.gatorconfig.json` with your DB connection string:
   ```json
   {
     "db_url": "postgres://user:password@localhost:5432/gator?sslmode=disable"
   }
   ```
3. Run migrations:
   ```
   cd sql/schema
   goose postgres "postgres://user:password@localhost:5432/gator?sslmode=disable" up
   ```
4. Build/install:
   ```
   go install
   ```

## Commands

- `register <name>` — create a new user and set them as current
- `login <name>` — set the current user
- `reset` — delete all users
- `users` — list all users
- `addfeed <name> <url>` — add a new RSS feed for the current user
- `feeds` — list all feeds and who added them
- `agg` — fetch a single feed and print the parsed result (for testing feed parsing)
