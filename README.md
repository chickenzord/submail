# submail

[![Tests](https://github.com/chickenzord/submail/actions/workflows/go.yml/badge.svg)](https://github.com/chickenzord/submail/actions/workflows/go.yml)

Virtual inbox router for AI agents.

## Overview

Submail connects to one or more real email inboxes via IMAP and re-exposes them as a REST API with per-agent access control. Each agent gets their own API token scoped to one or more configured addresses that all deliver into the same monitored inbox.

The addresses can be set up in any way your mail provider supports — plus-addressing (e.g. `bot+agent1@example.com`) is a common and convenient option, but fully separate aliases (e.g. `agent1@example.com`, `agent2@example.com`) pointing to the same inbox work just as well.

## Usage

### Server

```bash
submail server [--config ~/.config/submail/server.yaml]
```

Config can also be set via `SUBMAIL_CONFIG` env var.

#### Recommended setup with Docker Compose

```yaml
# docker-compose.yml
services:
  submail:
    image: ghcr.io/chickenzord/submail:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/etc/submail/config.yaml:ro
      - submail-data:/data
    environment:
      SUBMAIL_CONFIG: /etc/submail/config.yaml
      # Sensitive values can be injected as env vars instead of in config.yaml:
      # SUBMAIL_IMAP_PASSWORD: ...
      # SUBMAIL_AGENT_AGENT1_TOKEN: ...

volumes:
  submail-data:
```

```bash
docker compose up -d
```

See [`config.example.yaml`](config.example.yaml) for the full config reference. The `storage.path` in `config.yaml` should point inside the mounted volume (e.g. `/data/submail.db`).

### Client

#### Installation

**Homebrew (macOS / Linux):**
```bash
brew install chickenzord/tap/submail
```

**Go install:**
```bash
go install github.com/chickenzord/submail/cmd/submail@latest
```

**Binary download:** grab the archive for your platform from the [releases page](https://github.com/chickenzord/submail/releases).

#### Basic usage

```bash
submail inbox list
submail inbox get <id>
```

Client flags (or env vars):

| Flag | Env var | Description |
|---|---|---|
| `--url` | `SUBMAIL_URL` | Submail server URL |
| `--token` | `SUBMAIL_TOKEN` | Bearer token |

Use profiles to avoid repeating flags:

```bash
submail profile set myagent --url http://localhost:8080 --token <token>
export SUBMAIL_PROFILE=myagent
submail inbox list
```

#### Agent skill

The repo ships a `submail-client` skill that teaches AI agents (pi, OpenClaw-compatible) how to use the CLI — listing messages, fetching full content, pagination, error handling, and more.

Install it into your agent's skills directory:

```bash
# pi
cp -r skills/submail-client ~/.pi/skills/

# or clone directly
git clone https://github.com/chickenzord/submail /tmp/submail
cp -r /tmp/submail/skills/submail-client ~/.pi/skills/
```

Once installed, agents will automatically discover it and gain the ability to read from a Submail inbox.

## Configuration

See [`config.example.yaml`](config.example.yaml) for a full example.

Sensitive values can be supplied via environment variable or a file:

| Field | Env var | File variant |
|---|---|---|
| `imap.password` | `SUBMAIL_IMAP_PASSWORD` | `SUBMAIL_IMAP_PASSWORD__FILE` |
| `agents[*].token` | `SUBMAIL_AGENT_<ID>_TOKEN` | `SUBMAIL_AGENT_<ID>_TOKEN__FILE` |

## Mail Routing

Each mail is routed based on a single recipient address — whichever configured address it was delivered to. This means:

- Only the `To:` delivery address is used for routing; `Cc:` and `Bcc:` recipients are not considered.
- If a mail is addressed to multiple aliases belonging to the same agent, it will only appear once in their inbox (under whichever alias was recorded at ingest time).

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
