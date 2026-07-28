# Single-process Service: HTTP and aggregation in one process

`gator serve` runs one long-running process that both serves the REST API and
runs the aggregation loop as an in-process goroutine. We chose this over a
separate worker process (and over the previous model, where `serve` supervised
`agg` as a child process with crash-restart backoff) because one deployable unit
is simpler to run and the two workloads share the same DB layer anyway.

## Consequences

- The `internal/supervisor` package is deleted. Process-level restarts are the
  deployment environment's job (systemd, Docker); the aggregation goroutine
  recovers and logs internally instead of relying on process death + respawn.
- `agg` remains as a manual foreground loop for debugging, not for deployment.
- If fetch load ever needs independent scaling, splitting a worker out is the
  reversal path — the scraper package boundary keeps that possible.
