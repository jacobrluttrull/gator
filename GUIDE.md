# gator — quickstart

gator collects RSS feeds for you. You tell it which feeds to follow, it checks
them on a timer, and you read the new posts from your terminal — or over HTTP if
you'd rather have another program read them.

[README.md](README.md) is the short version: install, command list, endpoint
list. This page walks you through actually using it.

- [Setup](#setup)
- [Your first five minutes](#your-first-five-minutes)
- [Things you'll want to do](#things-youll-want-to-do)
- [Using gator from a script](#using-gator-from-a-script)
- [Working on gator itself](#working-on-gator-itself)

---

## Setup

You need [Go](https://go.dev/doc/install) 1.26+, [Postgres](https://www.postgresql.org/download/),
and [goose](https://github.com/pressly/goose) (which creates the tables).

**1. Install gator.**

```bash
go install github.com/jacobrluttrull/gator/cmd/gator@latest
```

The binary lands in `$HOME/go/bin`. If `gator` isn't found afterward, that
folder isn't on your `PATH`.

**2. Make a database.**

```bash
createdb gator
```

**3. Tell gator how to reach it.** Create `~/.gatorconfig.json` — in your home
folder, not in the project folder:

```json
{
  "db_url": "postgres://user:password@localhost:5432/gator?sslmode=disable"
}
```

Swap in your own Postgres username and password. You don't add
`current_user_name` yourself; gator writes it when you register or log in.

**4. Create the tables.**

```bash
goose -dir sql/schema postgres "postgres://user:password@localhost:5432/gator?sslmode=disable" up
```

Same connection string as step 3. Run this from a clone of the repo, since it
needs the `sql/schema` folder.

---

## Your first five minutes

```bash
gator register alice
```

Creates a user and makes you that user. There's no password — the CLI assumes
anyone who can read your config could reach the database anyway.

```bash
gator addFeed "Hacker News" "https://news.ycombinator.com/rss"
```

Adds the feed and follows it for you in one step. Quote both arguments.

```bash
gator agg 1m
```

This is the part people miss: **posts don't exist until something fetches
them.** `agg` fetches one feed, waits a minute, fetches the next, and keeps
going. Leave it running in its own terminal window. `Ctrl+C` stops it.

Give it a moment, then in a *second* terminal:

```bash
gator browse 10
```

Ten newest posts from feeds you follow. If you get nothing, `agg` probably
hasn't come around to your feed yet — wait for the interval and try again.

---

## Things you'll want to do

**Read what's new.**

```bash
gator browse           # 2 newest posts (the default)
gator browse 20        # 20 newest
gator browse 5 asc     # 5 oldest first
gator browse 5 asc 2   # page 2 of that — posts 6 through 10
```

The three arguments are positional and in that order: how many, direction, page.
To ask for a page you have to give all three.

**Find a post you half-remember.**

```bash
gator search postgres      # up to 5 matches
gator search postgres 20   # up to 20
```

It's a fuzzy match on post titles, so near-misses and typos still turn things
up. It only searches feeds you follow.

**Save something for later.**

```bash
gator bookmark "https://example.com/some-post"
gator bookmarks                                  # everything you've saved
gator unbookmark "https://example.com/some-post"
```

Bookmarks are keyed by the post's URL, which you can copy out of `browse`.

**Add and manage feeds.**

```bash
gator feeds                                   # every feed anyone has added
gator follow "https://example.com/rss"        # follow one that already exists
gator following                               # what you follow
gator unfollow "https://example.com/rss"
```

Feeds are shared by everyone; follows are yours. If a feed is already in the
list, `follow` it — `addFeed` would complain that the URL is taken.

**Switch users.**

```bash
gator users          # everyone, with a marker on you
gator login bob      # become bob
```

`login` takes no password. It just records who you're acting as.

**Start over.** `gator reset` deletes every user — and with them every feed,
follow, post, and bookmark. There's no confirmation prompt and no undo.

---

## Using gator from a script

If you want another program reading your posts, run the HTTP API instead of
`agg`. `serve` does both jobs at once: it answers requests *and* runs the same
fetch loop in the background.

**1. Give yourself a password.** The API needs one; the CLI never did.

```bash
gator setpassword hunter2
```

This also cancels any API keys you'd been using, so anything already talking to
the API will need a new one.

**2. Start the server.**

```bash
gator serve -port 8080 -interval 1m
```

Both flags are optional — that's what you get by default. They're flags, so
`gator serve 1m` is an error, not a shortcut. `Ctrl+C` shuts down the server and
the fetch loop together.

**3. Trade your password for a key.**

```bash
curl -s localhost:8080/v1/login \
  -d '{"name":"alice","password":"hunter2"}'
# {"api_key":"8f3ca91e..."}
```

Save that key. It's shown once and never again — the database only keeps a hash
of it.

**4. Use the key on every other request.**

```bash
curl -s localhost:8080/v1/posts -H "Authorization: ApiKey 8f3ca91e..."
```

The header has to read exactly `ApiKey <key>`. `Bearer` won't work and neither
will lowercase `apikey` — both come back as a plain 401.

### The endpoints

Everything lives under `/v1`. Everything except register and login needs the
key. Errors always come back as `{"error": "..."}`.

| Method | Path | You send | You get |
| --- | --- | --- | --- |
| POST | `/v1/register` | `{"name","password"}` | the new user |
| POST | `/v1/login` | `{"name","password"}` | `{"api_key"}` |
| GET | `/v1/feeds` | — | every feed |
| POST | `/v1/feeds` | `{"name","url"}` | the feed + your follow |
| GET | `/v1/follows` | — | what you follow |
| POST | `/v1/follows` | `{"url"}` | the new follow |
| DELETE | `/v1/follows` | `{"url"}` | nothing (204) |
| GET | `/v1/posts` | `?limit=` | posts from feeds you follow |
| GET | `/v1/bookmarks` | — | your bookmarks |
| POST | `/v1/bookmarks` | `{"url"}` | the new bookmark |
| DELETE | `/v1/bookmarks` | `{"url"}` | nothing (204) |
| GET | `/v1/search` | `?q=` and `?limit=` | matching posts |
| DELETE | `/v1/keys` | — | nothing (204) |

`DELETE /v1/keys` cancels **all** of your keys at once. That's the thing to call
if you think a key leaked.

`reset` and `users` are deliberately CLI-only. There's no way to wipe the
database over HTTP.

### Trying it without writing any code

`api-tests/` has the whole sequence — register, login, add a feed, follow,
browse, bookmark, search, revoke — with the API key filled in for you
automatically.

```bash
./api-tests/run.sh          # plain curl and bash, needs nothing installed
NAME=bob PASSWORD=s3cret ./api-tests/run.sh
```

It prints a pass/fail table and exits non-zero if something's broken, so it
doubles as a quick check that a change didn't break anything.

There are also `api-tests/gator.jetbrains.http` (GoLand, IntelliJ) and
`api-tests/gator.http` (VS Code REST Client) if you'd rather click through the
requests. Edit the name and password at the top and run them top to bottom. In
Zed, use `run.sh` — its `.http` extension only colors the file, it can't send
anything.

---

## Working on gator itself

```bash
goose -dir sql/schema postgres "$DB_URL" up   # apply new migrations
sqlc generate                                  # after editing anything in sql/queries/
go build ./...                                 # check it still compiles
```

Never edit `internal/database/` by hand — it's generated. Change the `.sql` file
and re-run `sqlc generate`.

Tests that touch the database need their own database, separate from your real
one, because the tests empty the tables between runs:

```bash
createdb gator_test
goose -dir sql/schema postgres "postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" up
GATOR_TEST_DB_URL="postgres://postgres:postgres@localhost:5432/gator_test?sslmode=disable" go test ./...
```

Without `GATOR_TEST_DB_URL` those tests skip instead of failing, so a clean
`go test ./...` doesn't necessarily mean they ran.

More reading: [CONTEXT.md](CONTEXT.md) for what the words mean,
[docs/adr/](docs/adr/) for why the design is the way it is,
[EXTENDING.md](EXTENDING.md) for ideas not built yet, and
[CLAUDE.md](CLAUDE.md) for the repo's conventions.
