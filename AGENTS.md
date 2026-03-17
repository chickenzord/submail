# Submail — Agent Guide

## Overview

Submail is a Go server that monitors a shared IMAP inbox and re-exposes it as a per-agent REST API. Each agent authenticates with a Bearer token and sees only messages addressed to its configured email aliases.

## Project Map

```
cmd/submail/
  main.go          — entry point, wires cobra root command
  server.go        — `submail server` command
  client.go        — shared HTTP client + profile resolution for CLI commands
  inbox.go         — `submail inbox list/get` commands
  profile.go       — `submail profile set/show/list/delete` commands

internal/
  config/
    config.go      — load, default, env override, secret resolution, validate
  imap/
    ingester.go    — IMAP polling loop, server-side address filtering, dedup, storage
  storage/
    storage.go     — Store interface (SaveMessage, Get, List, Count, …)
    sqlite.go      — SQLite implementation via modernc.org/sqlite (no CGO)
  api/
    server.go      — Echo server setup, route registration
    middleware.go  — Bearer token auth, agent context injection
    handlers.go    — GET /api/v1/inbox/mails, GET /api/v1/inbox/mails/:id
    admin.go       — session-based admin auth, admin route registration
    admin_handlers.go — admin web UI handlers (HTML)
    model.go       — Mail / ListMailsResponse API types

skills/
  submail-client/SKILL.md  — OpenClaw-compatible agent skill for the CLI
```

## Non-Obvious Behaviors

### Config loading order
`Load()` applies steps in strict order: parse YAML → `setDefaults()` → `applyEnvOverrides()` → `resolveSecrets()` → `validate()`. This means env vars always win over YAML values, and secrets are resolved after all overrides are applied.

### Secret resolution (`__FILE` pattern)
For any secret field (IMAP password, admin password, agent tokens) the lookup order is:
1. `<ENV>__FILE` — read from the file path in that env var (whitespace trimmed)
2. `<ENV>` — use the env var value directly
3. YAML value as fallback

### Agent ID must be alphanumeric
`config.validate()` rejects agent IDs with non-alphanumeric characters. This is because the env var override pattern `SUBMAIL_AGENT_<ID>_TOKEN` uppercases the ID directly — special characters would produce unparseable variable names.

### IMAP filtering is server-side
The ingester builds a `SEARCH TO` IMAP query (nested `OR` for multiple addresses) so only relevant messages are fetched from the server. Only the first `To` address on a message is stored and used for routing; `Cc`/`Bcc` are ignored entirely.

### Deduplication key is the email `Message-ID` header
`SaveMessage` is an upsert keyed on the email `Message-ID`. When a message has no `Message-ID` (unusual but valid), the ingester synthesises `uid-<uid>@<host>` so restarts remain idempotent.

### MIME parse errors are soft
`parseBodies()` stops iterating MIME parts on any error but returns whatever text/HTML it already extracted — no hard failure. A malformed attachment won't prevent the message from being stored.

### IMAP reconnect uses exponential backoff
On any session error the ingester waits and reconnects: starts at 2 s, doubles each attempt, caps at 5 min. A clean context cancellation exits without retrying.

### Admin uses session auth, not Bearer tokens
The admin web UI (`/admin`) uses an HTTP session cookie, not the agent Bearer token scheme. Admin routes are also excluded from Echo's access log middleware. Enabling admin requires `server.admin.password` to be set — `validate()` enforces this.

### SQLite implementation is CGO-free
Storage uses `modernc.org/sqlite`, a pure-Go SQLite port. No CGO, no system sqlite library needed. This is why the Dockerfile uses a plain `scratch`/`alpine` base without extra dependencies.
