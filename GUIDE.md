# gator — the long guide

Everything you need to work on, run, and debug gator. [README.md](README.md) is
the front door (install, command list, endpoint list); this is the "how does it
actually work" document.

- [Mental model](#mental-model)
- [The three moving parts](#the-three-moving-parts)
- [How a CLI command runs](#how-a-cli-command-runs)
- [How an API request runs](#how-an-api-request-runs)
- [The data model](#the-data-model)
- [Aggregation: how posts actually show up](#aggregation-how-posts-actually-show-up)
- [Auth: passwords and API keys](#auth-passwords-and-api-keys)
- [API reference](#api-reference)
- [Development workflow](#development-workflow)
- [Debugging playbook](#debugging-playbook)
- [Traps worth knowing about](#traps-worth-knowing-about)

---

## Mental model

gator is a small app with one database and two front doors.

```
                 ┌──────────────────────────────┐
   terminal ───▶ │  CLI  (gator <command>)      │──┐
                 └──────────────────────────────┘  │
                                                   ├──▶  Postgres
                 ┌──────────────────────────────┐  │
   HTTP    ───▶  │  API  (gator serve, /v1/...) │──┘
                 └──────────────────────────────┘
                            │
                            └─ also runs the aggregation loop
                               (fetch RSS → save new posts)
```

The two front doors are deliberately different in how much they trust you:

**The CLI is trusted.** It opens a Postgres connection with the URL from
`~/.gatorconfig.json` and does whatever you ask. There is no password on the CLI
because there'd be no point — anyone who can read that config file can already
connect to Postgres directly and do anything. `gator login alice` doesn't
authenticate; it just writes `alice` into the config as "who I'm acting as".

**The API is not trusted.** It's the only thing exposed to a network, so it
carries all the credential machinery: bcrypt passwords, per-user API keys, and a
middleware that rejects anything without a valid key.

That split is [ADR-0001](docs/adr/0001-two-surface-trust-model.md), and it
explains a bunch of things that otherwise look like bugs:

- `login` not asking for a password — intentional, don't "fix" it.
- `reset` and `users` existing on the CLI but having no HTTP endpoint — admin
  verbs stay on the trusted side.
- A user created with `gator register` having a `NULL` password_hash and being
  unable to log in over HTTP until someone runs `setpassword`. We call these
  **CLI-only users**.

The vocabulary the codebase uses consistently (Feed vs Following, API login vs
CLI login, Revocation, Service) is defined in [CONTEXT.md](CONTEXT.md). Worth
five minutes — the words are load-bearing in the code and comments.

---

## The three moving parts

### 1. `cmd/gator` — the entrypoint

About 60 lines. It reads the config, opens the DB, builds one map of command
name → handler function, and dispatches `os.Args[1]` through it. If the command
returns an error, `log.Fatal` prints it and exits 1. That's the whole program.

Adding a command means adding one line to that map literal in
`cmd/gator/main.go`. Not a loop, not repeated calls — the map literal is the
registry.

### 2. `internal/*` — the actual work

The packages are layered so that dependencies only point one way:

```
cli        (State, Command, Commands, LoggedIn)   ← depends on nothing else here
auth       (bcrypt, API keys)                     ← standalone
feed       (fetch + parse RSS)                    ← standalone
database   (sqlc-generated)                       ← standalone

api        → auth, database
scraper    → cli, feed, database
handlers   → cli, scraper, api, database
cmd/gator  → everything
```

If you find yourself wanting `cli` to import `handlers`, stop — that's the
direction that creates a cycle.

### 3. Postgres

Schema lives in `sql/schema/` as goose migrations. Queries live in
`sql/queries/`, one query per file, and sqlc turns them into typed Go in
`internal/database/`. **Never hand-edit `internal/database/`** — it gets
regenerated and your edit vanishes.

---

## How a CLI command runs

Trace `gator browse 5` from keypress to output:

1. **`main()`** reads `~/.gatorconfig.json` into a `config.Config` (`db_url`,
   `current_user_name`), opens the Postgres handle, and wraps it in
   `database.New(db)` — the sqlc query struct.
2. It builds a `cli.State{Config, DB}`. State is the bag that gets handed to
   every handler; it's how a handler reaches the config and the database.
3. It looks up `"browse"` in `cmds.Handlers` and calls it with
   `cli.Command{Name: "browse", Args: ["5"]}`. Unknown name → `unknown command: x`.
4. `browse` was registered as `cli.LoggedIn(handlers.Browse)`. That wrapper
   (`internal/cli/cli.go:35`) runs first: it calls `GetUser` with
   `Config.CurrentUserName` and injects the resulting `database.User` into the
   handler. This is why no handler ever calls `GetUser` itself.
5. `handlers.Browse` parses its positional args, runs `GetPostsForUser`, and
   prints title + URL per post.

The two handler signatures, and that's the whole pattern:

```go
func X(s *cli.State, cmd cli.Command) error                      // no user needed
func X(s *cli.State, cmd cli.Command, user database.User) error  // wrapped in cli.LoggedIn at registration
```

**Positional args are parsed by hand and are strict about order.** `browse
[limit] [asc] [page]` reads arg 0 as the limit, arg 1 as literally the string
`asc` (anything else means descending), arg 2 as a 1-based page number. So
`gator browse asc` fails with "invalid limit" — the `asc` landed in the limit
slot. `serve` is the exception: it uses a real `flag.FlagSet`, so it takes
`-port` and `-interval` and rejects positional args outright.

---

## How an API request runs

Trace `GET /v1/posts?limit=10` with an `Authorization: ApiKey abc...` header:

1. **`gator serve`** (`internal/handlers/serve.go`) parses `-port`/`-interval`,
   starts the aggregation goroutine, and starts an `http.Server` whose handler
   is `api.New(s.DB)`.
2. **`api.New`** (`internal/api/api.go:27`) builds a route *table* — a slice of
   `{method, path, handler}` — then registers each entry on a `http.ServeMux`
   using Go 1.22+ method patterns (`"GET /v1/posts"`). It also registers `"/"`
   as a catch-all.

   The table exists rather than a run of `mux.HandleFunc` calls because the
   catch-all needs the same data: `handleUnmatched` answers an unknown path with
   a JSON 404, and a known path with the wrong verb with a JSON 405 *plus a
   correct `Allow` header*. Registering `"/"` takes over the mux's own plain-text
   405, so the table is what keeps `Allow` honest.
3. The mux matches `GET /v1/posts` → `loggedIn(db, handleListPosts(db))`.
4. **`loggedIn`** (`internal/api/middleware.go:21`) is the HTTP twin of
   `cli.LoggedIn`. It parses the `Authorization` header, SHA-256s the key, looks
   up the user by hash, and injects the user. It **fails closed**: missing
   header, malformed header, and unknown key all produce the exact same
   `401 {"error":"missing or invalid API key"}`, so an attacker can't tell a real
   key from a fake one by the error text.
5. **`handleListPosts`** reads `limit` via the shared `limitParam` helper
   (default 2, rejects non-positive and out-of-int32-range values with a 400),
   queries, and responds.
6. **`respondJSON` / `respondError`** write the response. Errors are always the
   same shape: `{"error": "message"}`.

Two shared helpers in `api.go` are worth knowing because they write their own
error responses and return a `bool` — check the bool and return, don't try to
respond again:

- `urlFromBody(w, r)` — decodes `{"url": ...}`; 400 if missing or unparseable.
- `limitParam(w, r, default)` — reads `?limit=`; 400 if present but not a
  positive int32.

### Shutdown

`serve` catches SIGINT/SIGTERM through `signal.NotifyContext`. On signal it
cancels the context (which stops the aggregation goroutine at its next select),
calls `srv.Shutdown` with a 10-second grace window, waits for the goroutine to
finish, and returns. If `ListenAndServe` dies on its own — port already in use,
usually — it cancels the loop and returns that error immediately.

There is no process supervisor. Restarts are systemd's or Docker's job
([ADR-0002](docs/adr/0002-single-process-service.md)); the loop recovers from its
own panics instead of relying on the process dying.

---

## The data model

Five tables. Every one has `id UUID` primary key plus `created_at` /
`updated_at`, and all timestamps are generated in Go with `time.Now().UTC()`,
never with SQL `NOW()`.

```
users ──┬──< feeds          (user_id = who added it)
        ├──< feed_follows >── feeds
        ├──< post_bookmarks >── posts
        └──< api_keys

feeds ──< posts
```

| Table | Holds | Notable constraints |
| --- | --- | --- |
| `users` | name, `password_hash` (nullable) | `name` unique. Null hash = CLI-only user |
| `feeds` | name, url, `user_id`, `last_fetched_at` | `url` unique (added late — see below) |
| `feed_follows` | user_id, feed_id | `unique(user_id, feed_id)` — can't follow twice |
| `posts` | title, url, description, published_at, feed_id | `url` **globally** unique |
| `post_bookmarks` | user_id, post_id | `unique(user_id, post_id)` |
| `api_keys` | `key_hash`, user_id | `key_hash` unique. Only the hash is stored |

Every foreign key is `on delete cascade`. Deleting a user takes their feeds,
follows, bookmarks, and API keys with them — and deleting their feeds takes those
feeds' posts. This is why `gator reset` (a bare `DELETE FROM users`) empties the
entire database.

### Feeds are shared, follows are personal

A feed exists **once**, globally, no matter how many people follow it. Whoever
runs `addFeed` first creates it; everyone else runs `follow <url>`. The
aggregator fetches each feed once regardless of follower count.

So "my feeds" always means "feeds I follow", never "feeds I created". The
`user_id` column on `feeds` is only a record of who added it — `gator feeds`
prints it as "added by".

### `posts.url` is globally unique

Not per-feed — globally. If two feeds carry the same article URL, it's stored
once, under whichever feed reached it first. The scraper relies on this: it
attempts every insert and silently skips Postgres error `23505` (unique
violation), which is exactly "we already have this post".

### The dedup + unique-constraint migration pair

`feeds.url` wasn't unique originally, so an older database can hold two feeds on
the same URL. Adding the constraint would fail on such a database, and goose
stops at the first failing migration — so the cleanup has to run *before* the
constraint, not after:

```
20260729041443_dedup_feed_urls.sql       ← collapses duplicates (oldest row wins)
20260729041444_add_feed_url_unique.sql   ← adds the constraint
```

Those versions are hand-written, not `goose create`d, precisely to slot in
ahead. If your database already applied the later one, plain `goose up` refuses
with *"found 1 missing migrations"*; you need a one-time
`goose ... -allow-missing up`. `internal/testsupport/migration_test.go` guards
this ordering so nobody accidentally reorders it.

---

## Aggregation: how posts actually show up

The loop is `scraper.Run` (`internal/scraper/run.go`), shared by both `gator agg
<interval>` and `gator serve -interval`:

1. Scrape **immediately** (not after the first tick — so you see activity right
   away).
2. Then scrape once per interval until the context is cancelled.
3. A failed scrape is logged and skipped — never fatal to the loop.
4. Panics inside a scrape are recovered and turned into a logged error, because
   there's no supervisor to restart the process.
5. A non-positive interval returns an error instead of panicking on
   `time.NewTicker`.

One scrape (`scraper.Scrape`) does this:

1. `GetNextFeedToFetch` — `ORDER BY last_fetched_at ASC NULLS FIRST LIMIT 1`.
   **One feed, the least recently fetched, never-fetched feeds first.**
2. `MarkFeedFetched` — stamps it *before* fetching. So a feed that fails still
   goes to the back of the queue and won't be retried in a tight loop.
3. `feed.Fetch` — GET with `User-Agent: gator`, parse the XML into
   `RSSFeed`/`RSSItem`, HTML-unescape titles and descriptions.
4. For each item, `CreatePost` — skipping duplicates (`23505`) silently, logging
   any other insert error and continuing.

**The single most important consequence:** one tick fetches *one* feed. With 10
feeds and `-interval 1m`, each individual feed gets refreshed every 10 minutes,
not every minute. If posts seem slow to appear, that's usually why — not a bug.

Publish dates are parsed by `ParsePublishedAt`, which tries RFC1123Z, RFC1123,
RFC3339, RFC822Z, RFC822 in order and returns the zero time if none match. A feed
using some other date format gets posts with `published_at` of year 1, which
sorts them to the bottom of `browse` forever.

---

## Auth: passwords and API keys

### Passwords

- bcrypt at default cost, via `internal/auth`.
- Hard limit of 72 bytes (bcrypt's own limit). Longer returns
  `ErrPasswordTooLong`, which both surfaces map to "you sent something invalid" —
  a 400 over HTTP, a plain error message on the CLI. It's user input, not a 500.
- An empty password is rejected by `setpassword`. That's deliberate: an empty
  password would hash to a perfectly valid bcrypt entry, and then an API login
  sending no password would match it.
- Only the CLI can set a password (`gator setpassword <new>`). There's no
  old-password check, because the CLI is already the trusted surface.

### API keys

- 32 random bytes, hex-encoded, generated at API login.
- **Only the SHA-256 hash is stored.** The plaintext key is returned exactly
  once, in the login response. Lose it and you log in again.
- A user can hold several at once — one per client. Logging in again doesn't
  invalidate anything.
- Keys never expire ([ADR-0001](docs/adr/0001-two-surface-trust-model.md)), so
  **revocation is the only way a key ever stops working**. Two ways to revoke:
  `DELETE /v1/keys` (kills all of the caller's keys, including the one making
  the request), or `gator setpassword` (a new password must not leave old keys
  alive).

The `setpassword` revocation is worth knowing where it lives: it is **in the
SQL**, not the Go handler. `sql/queries/setuserpassword.sql` is a data-modifying
CTE that updates the password and deletes the user's `api_keys` rows in a single
statement, so there's no window where the password has changed but old keys still
authenticate. If you go looking for a `DeleteAPIKeysForUser` call in
`handlers/auth.go`, you won't find one — that's not a missing feature.

### Header format

Exactly `Authorization: ApiKey <key>` — two space-separated tokens, exact
capitalization on `ApiKey`. `Bearer`, lowercase `apikey`, or a key with a space
in it all get the same 401.

---

## API reference

Base URL `http://localhost:8080` by default. Everything below `/v1`. Every
endpoint except `register` and `login` requires the header. Errors are always
`{"error": "..."}`.

JSON field names mirror the database rows (`created_at`, `user_id`, ...) rather
than being camelCased.

### `POST /v1/register`

```bash
curl -s localhost:8080/v1/register -d '{"name":"alice","password":"hunter2"}'
```

→ `201` with the user (`id`, `created_at`, `updated_at`, `name` — never the
password hash). `400` if name or password is missing or the password is over 72
bytes. `409` if the name is taken.

Note this is *not* the same as `gator register`, which creates a CLI-only user
with no password.

### `POST /v1/login`

```bash
curl -s localhost:8080/v1/login -d '{"name":"alice","password":"hunter2"}'
```

→ `200 {"api_key":"<64 hex chars>"}`. This is the only time you see the key.

→ `401 {"error":"invalid username or password"}` for a wrong password, an unknown
user, a missing password field, *and* a CLI-only user who has no password set.
All four are the same response on purpose — the endpoint doesn't leak which users
exist or which ones have passwords.

### `GET /v1/feeds`

Every feed in the shared pool: `[{"feed_name","feed_url","username"}]`, where
`username` is who added it. Empty pool returns `[]`, not `null`.

### `POST /v1/feeds`

```bash
curl -s localhost:8080/v1/feeds -H "Authorization: ApiKey $KEY" \
  -d '{"name":"Hacker News","url":"https://news.ycombinator.com/rss"}'
```

Adds the feed *and* follows it in one step, like the CLI's `addFeed`. →`201`
with both objects nested:

```json
{"feed": {...}, "feed_follow": {...}}
```

`409` if a feed with that URL already exists — use `POST /v1/follows` instead.

### `GET|POST|DELETE /v1/follows`

`GET` lists your follows. `POST`/`DELETE` take `{"url": ...}` — the feed's URL,
not an id:

```bash
curl -s localhost:8080/v1/follows -H "Authorization: ApiKey $KEY" \
  -d '{"url":"https://news.ycombinator.com/rss"}'

curl -s -X DELETE localhost:8080/v1/follows -H "Authorization: ApiKey $KEY" \
  -d '{"url":"https://news.ycombinator.com/rss"}'
```

`POST` → `201` with the follow (including `user_name` and `feed_name`), `404` if
no feed has that URL, `409` if you already follow it. `DELETE` → `204` with no
body, and it's idempotent — unfollowing something you don't follow still returns
`204`.

### `GET /v1/posts`

```bash
curl -s "localhost:8080/v1/posts?limit=20" -H "Authorization: ApiKey $KEY"
```

Posts from feeds you follow, newest `published_at` first. **`limit` defaults to
2** (matching the CLI's `browse` default) — if you forget it, two posts is not a
bug. There's no paging on this endpoint; the CLI's `browse` has the offset
support.

### `GET|POST|DELETE /v1/bookmarks`

Same `{"url": ...}` shape as follows, but the URL is a *post's* URL. `GET`
returns the bookmarked posts themselves (newest bookmark first), not bookmark
join rows. `POST` → `201` with the bookmark (including `post_title`,
`post_url`), `404` for an unknown post URL, `409` if already bookmarked.
`DELETE` → `204`.

### `GET /v1/search`

```bash
curl -s "localhost:8080/v1/search?q=postgres&limit=10" -H "Authorization: ApiKey $KEY"
```

Fuzzy trigram search over post titles from feeds you follow. Each result is a
post plus `"sim"` — the similarity score, ordered highest first. `limit` defaults
to 5. A blank or whitespace-only `q` is a `400` (the CLI can't be invoked without
a term, but a query string can be empty).

### `DELETE /v1/keys`

```bash
curl -s -X DELETE localhost:8080/v1/keys -H "Authorization: ApiKey $KEY"
```

Revokes **every** API key you hold, including the one making the request. That's
the point: if you think a key leaked but don't know which one, you can kill them
all. → `204`.

### Errors you'll hit generally

| Status | When |
| --- | --- |
| 400 | Bad JSON body, missing required field, bad `limit`, blank `q` |
| 401 | Missing/malformed/unknown API key; bad credentials at login |
| 404 | Unknown path, or a `url` in the body that matches no feed/post |
| 405 | Right path, wrong verb — comes with an `Allow` header listing the real ones |
| 409 | Uniqueness conflict: name taken, feed URL exists, already following/bookmarked |
| 500 | Something failed in the DB layer; check the server log |

---

## Development workflow

### Changing the schema

Always a new migration file — never edit one that's already been applied, even
if it's only applied on your own machine on an unmerged branch.

```bash
goose -dir sql/schema create add_something sql
# edit the generated file: -- +goose Up / -- +goose Down
goose -dir sql/schema postgres "$DB_URL" up
```

New Postgres extensions and indexes (`pg_trgm`, the GIN index) are schema
changes, so they're migrations too — not queries.

If a new constraint could be violated by rows that already exist, the cleanup
goes in a **separate migration versioned before** the constraint, hand-named
rather than `goose create`d. See the feed-url pair described above.

### Changing queries

```bash
# edit or add a file in sql/queries/ — one query per file, filename matching the
# query name (getfeedbyurl.sql → GetFeedByUrl)
sqlc generate
go build ./...
```

Then **read the generated `.sql.go`** before writing the calling code. sqlc's
parameter naming surprises people:

- A param wrapped in a SQL function gets named after the *function*. In
  `searchposts.sql`, `similarity(lower(title), lower($2))` produces a struct
  field called `Lower`, not `Query` or `Term`. That's why the search handlers
  say `Lower: query`.
- Reusing the same `$n` for two columns collapses them into **one** Go field.
  `markfeedfetched.sql` sets `updated_at = $2, last_fetched_at = $2` and gets a
  single `UpdatedAt` param, not two.

### Tests

```bash
go test ./...   # unit tests only; the integration tests skip
```

`internal/api` and `internal/handlers` are integration tests against a real
Postgres. They skip unless `GATOR_TEST_DB_URL` is set:

```bash
createdb gator_test
goose -dir sql/schema postgres "postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" up
GATOR_TEST_DB_URL="postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" go test ./...
```

The shared harness is `testsupport.OpenTestDB`. Two things it does that matter:

- It runs `TRUNCATE users CASCADE` before each test. **Point it at a dedicated
  database.** Never your dev database.
- It takes a Postgres advisory lock on one pinned connection for the duration of
  each test, because `go test ./...` runs packages in parallel and both packages
  truncate the same database. The lock serializes them. If a test looks hung,
  it's probably queued behind another package — that wait is intentionally
  unbounded, while the truncate has a 30-second timeout.

Go test files live next to the package they test. There's no top-level `tests/`
directory and there can't usefully be one — a separate directory can't reach
unexported identifiers.

---

## Debugging playbook

Symptom → most likely cause, roughly in the order you'll hit them.

### "no such file or directory" / config errors on any command

`config.Read()` looks at `$HOME/.gatorconfig.json` and **nowhere else**. A
`.gatorconfig.json` sitting in the repo root is not read. Check the real one:

```bash
cat ~/.gatorconfig.json
```

### Every command errors with a Postgres connection message

`sql.Open` doesn't actually connect — it's lazy — so a bad `db_url` first
surfaces as a failure inside the handler, not at startup. Test it directly:

```bash
psql "$(jq -r .db_url ~/.gatorconfig.json)" -c 'select 1'
```

Local Postgres usually needs `?sslmode=disable` on the URL.

### `relation "users" does not exist` (or any table)

Migrations haven't been applied to *that* database. Check what goose thinks:

```bash
goose -dir sql/schema postgres "$DB_URL" status
```

### `goose up` says "found 1 missing migrations"

You have a database that applied `..._add_feed_url_unique` before the dedup
migration was slotted in ahead of it. One-time fix:

```bash
goose -dir sql/schema postgres "$DB_URL" -allow-missing up
```

### `scrape failed: sql: no rows in result set`, repeating every interval

`GetNextFeedToFetch` found no feeds at all. The database has zero feeds — add one
with `addFeed`. The loop logs and keeps going, which is correct behavior, but the
message doesn't spell out the cause.

### The aggregator is running but no new posts appear

Work down this list:

1. **One feed per tick.** With N feeds, any given feed is refreshed every
   N × interval. Check `select name, last_fetched_at from feeds order by
   last_fetched_at;` — is the feed you care about actually being reached?
2. **Are you following it?** `browse` and `/v1/posts` filter by *your* follows,
   not by what exists. `gator following` will tell you. Adding a feed via
   `addFeed` follows it automatically; a feed someone else added does not.
3. **Are the posts there at all?** `select count(*) from posts;` separates "not
   fetched" from "fetched but not visible to me".
4. **Bad publish dates.** `browse` sorts by `published_at` descending. A feed
   whose `pubDate` format isn't one of the five parsed layouts gets
   `0001-01-01`, so its posts sort to the very bottom. Check with
   `select title, published_at from posts order by published_at limit 5;`
5. **Duplicate URLs.** If the article already exists under another feed,
   `posts.url` being globally unique means it isn't stored again.

### `browse` prints exactly two posts

That's the default limit. `gator browse 20`. Same for `/v1/posts` — default
`limit` is 2.

### Search returns nothing for a word that's obviously in a title

This is the most confusing behavior in the app, and it isn't a broken index.

`similarity()` compares the **whole title** against the **whole search term**,
and pg_trgm's default threshold (`show_limit()`) is `0.3`. A short term against a
long title scores low no matter how exactly it appears inside it. Measured
against a real post titled *"More Tailscale tricks for your jailbroken Kindle"*:

| Term | `similarity()` | Matches? |
| --- | --- | --- |
| `kindle` | 0.152 | no — under 0.3 |
| `tailscale kindle` | 0.348 | yes |
| the full title | 1.000 | yes |

So this is fuzzy *title* matching — "find the post whose title looks roughly like
this" — not substring search. Use more of the title, not less. To check what a
given term would score:

```sql
select similarity(lower(title), lower('kindle')), title from posts;
```

If you want single-word matching, that's a query change, not a tuning knob:
`word_similarity()` (the `<%` operator) scores a term against the best-matching
*part* of the title, which is the behavior people usually expect here.

### `gator browse asc` → "invalid limit"

Positional args, fixed order: `browse [limit] [asc] [page]`. You want
`gator browse 10 asc`.

### `serve` exits immediately with a port error

`ListenAndServe` failed — usually the port is taken. `gator serve -port 8081`, or
find the squatter with `ss -ltnp | grep 8080`.

### `serve: unexpected argument "1m"`

`serve` takes flags only, not a positional interval (an older version did).
Use `gator serve -interval 1m`.

### Everything over HTTP returns 401

In order of likelihood:

1. Header format. It must be exactly `Authorization: ApiKey <key>` — not
   `Bearer`, not `apikey`.
2. The key was revoked. `DELETE /v1/keys` kills all of them, and
   `gator setpassword` kills them too, as part of the same SQL statement that
   changes the password. Log in again.
3. You're using the key against a different database than the one that issued it.

The 401 body is identical for all causes by design, so the error text won't
narrow it down. To confirm a key exists server-side:

```sql
select count(*) from api_keys where user_id = (select id from users where name='alice');
```

(You can't look up the key itself — only its SHA-256 hash is stored.)

### `POST /v1/login` returns 401 for a user you know exists

They're a CLI-only user: created with `gator register`, so `password_hash` is
NULL. Fix with `gator login alice && gator setpassword hunter2`. Check with:

```sql
select name, password_hash is null as cli_only from users;
```

### 404 on a `url` body, when the URL looks right

The lookup is an exact string match on `feeds.url` / `posts.url`. A trailing
slash, `http` vs `https`, or a `?utm_source=` suffix makes it a different URL.
Compare against what's actually stored: `select url from feeds;`

### A 500 from the API

The response body is deliberately vague ("couldn't list posts"). The real error
went to the server's log — look at the `gator serve` terminal.

### Tests fail with "GATOR_TEST_DB_URL not set" — or rather, skip

They skip silently by design. `go test ./...` passing does *not* mean the
integration tests ran. Set the env var (see above) if you want real coverage.

### Tests wipe data you cared about

`OpenTestDB` truncates `users CASCADE` — which cascades to everything. It only
ever touches `GATOR_TEST_DB_URL`, so the fix is to make sure that variable never
points at your dev database.

### A test hangs

Probably queued on the advisory lock behind another package's test. That wait is
unbounded on purpose. If nothing else is running, a stale session may still hold
it:

```sql
select pid, query from pg_stat_activity where query like '%pg_advisory_lock%';
```

### Changes to a `.sql` file don't take effect

`sqlc generate` isn't automatic. Run it, then `go build ./...`. And if you edited
something under `internal/database/` directly — that's generated code; the change
will be overwritten. Edit `sql/queries/` instead.

---

## Traps worth knowing about

- **`gator reset` deletes everything.** It's `DELETE FROM users`, and every FK
  cascades. No confirmation prompt.
- **`agg` is a debugging tool, not a deployment.** It runs in the foreground
  forever; Ctrl+C stops it. The real deployment is `serve`, which runs the same
  loop in-process ([ADR-0002](docs/adr/0002-single-process-service.md)).
- **The trigram index isn't actually used.** The GIN index is on plain `title`,
  but the search query wraps both sides in `lower()`, so Postgres can't use it.
  Fine at this data size; it would need a functional index to matter.
- **Search is whole-title fuzzy matching, not substring search.** A single word
  from a long title usually scores under the 0.3 threshold and returns nothing.
  See the playbook entry above — this surprises everyone once.
- **`internal/database/` is generated.** Saying it a third time because it's the
  easiest hour to lose in this repo.
- **`go install` needs the `/cmd/gator` suffix.** The module root has no `main`
  package.
- **Timestamps come from Go, not SQL.** If you write a query using `NOW()`,
  you've broken the convention and made the value untestable.
