# Gator

An RSS feed aggregator: users follow feeds, a background loop fetches new posts,
and users browse, search, and bookmark them. Two surfaces exist over one Postgres
database — a trusted local CLI and an authenticated REST API.

## Language

### Surfaces

**CLI**:
The trusted local surface. It talks directly to Postgres; possessing the DB URL is
its only credential, so it can act as any user.
_Avoid_: admin tool, local client

**API**:
The authenticated remote surface. Every request is tied to a user via an API key;
passwords gate this surface only.
_Avoid_: web service, backend

**Service**:
The single long-running process that serves the API and runs the aggregation loop.
_Avoid_: server, daemon, worker

### Auth

**API key**:
A long-lived credential issued at API login and presented on every API request to
identify the user. A user may hold several at once, one per client.
_Avoid_: token, session

**API login**:
Verifying a user's password and issuing an API key. Distinct from CLI login.
Additive: it never invalidates a key issued earlier.

**Revocation**:
Destroying all of a user's API keys at once — `DELETE /v1/keys` over the API, or as
part of a CLI `setpassword`. Keys never expire (ADR-0001), so revoking is the only
way one ever stops working.
_Avoid_: logout, expiry, rotation

**CLI login**:
Selecting which user the CLI acts as (no password involved).
_Avoid_: switch user

**CLI-only user**:
A user who has never set a password. They can act through the trusted CLI but
cannot API-login until a password is set.
_Avoid_: legacy user, locked account

### Feeds

**Feed**:
An RSS source in the shared global pool. Added once by some user, followable by
every user, fetched once regardless of follower count.
_Avoid_: subscription, source, channel

**Following**:
A user's personal subscription to a feed from the shared pool. "A user's feeds"
always means the feeds they follow, not feeds they created.
_Avoid_: owning a feed, my feeds (when meaning created-by-me)

### Aggregation

**Aggregation**:
The continuous background fetching of followed feeds and saving of their new posts.
_Avoid_: scraping, polling, syncing
