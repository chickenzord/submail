# submail

[![Tests](https://github.com/chickenzord/submail/actions/workflows/go.yml/badge.svg)](https://github.com/chickenzord/submail/actions/workflows/go.yml)

Virtual inbox router for AI agents.

## Overview

Submail connects to one or more real email inboxes via IMAP and re-exposes them as a REST API with per-agent access control. Multiple AI agents sharing a single email address via plus-addressing (e.g. `bot+agent1@example.com`) each get their own API token scoped to their aliased address.

## Usage

### Server

```bash
submail server [--config ~/.config/submail/server.yaml]
```

Config can also be set via `SUBMAIL_CONFIG` env var.

### Client

```bash
submail inbox list
submail inbox get <id>
```

Client flags (or env vars):

| Flag | Env var | Description |
|---|---|---|
| `--url` | `SUBMAIL_URL` | Submail server URL |
| `--token` | `SUBMAIL_TOKEN` | Bearer token |

## Configuration

See [`config.example.yaml`](config.example.yaml) for a full example.

Sensitive values can be supplied via environment variable or a file:

| Field | Env var | File variant |
|---|---|---|
| `imap.password` | `SUBMAIL_IMAP_PASSWORD` | `SUBMAIL_IMAP_PASSWORD__FILE` |
| `agents[*].token` | `SUBMAIL_AGENT_<ID>_TOKEN` | `SUBMAIL_AGENT_<ID>_TOKEN__FILE` |

## API

All endpoints require `Authorization: Bearer <token>`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/inbox/mails` | List mails (supports `?limit=` and `?offset=`) |
| `GET` | `/api/v1/inbox/mails/:id` | Get a mail by ID |

## Development

```bash
# Run tests
go test ./...

# Run server
submail server
```
