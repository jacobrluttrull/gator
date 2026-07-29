# Two-surface trust model: trusted CLI, authenticated API

Gator has two surfaces over one Postgres database. The CLI talks directly to the
DB and stays passwordless — possessing the DB URL in `~/.gatorconfig.json` is its
credential, and anyone holding it can already do anything to Postgres, so a CLI
password would be security theater. The API is the only network-exposed surface,
and it alone enforces credentials: bcrypt passwords at registration, and a
per-user API key (random 32-byte hex, stored as a SHA-256 hash) on every request.

## Considered Options

- Make the CLI an API client so one authenticated path serves everything —
  rejected: a full CLI rewrite, and local use would require the server running.
- Literal HTTP Basic / JWT for the API — rejected in favor of long-lived API
  keys: one bcrypt check at login, revocable per user, no expiry/refresh machinery.

## Consequences

- CLI `login <name>` remains "select a user", not authentication. Don't "fix" it.
- Existing users have a null `password_hash` (CLI-only users); they cannot
  API-login until the trusted CLI `setpassword` command sets one.
- Admin verbs (`reset`, `users`) live only on the trusted CLI surface, never HTTP.
