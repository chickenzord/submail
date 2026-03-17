# Submail

## Overview

Submail is a Go server application that acts as a **virtual inbox router for AI agents**. It connects to one or more real email inboxes via IMAP — monitoring them without marking messages as read — and re-exposes them as a REST API with per-agent access control.

## Problem It Solves

Multiple AI agents often share a single real email address through plus-addressing (e.g. `bot+agent1@example.com`, `bot+agent2@example.com`). These all land in the same physical inbox, making it impossible for each agent to independently and securely access only "their" messages.

Submail sits in front of that inbox and provides each agent with:
- Its own API key
- A virtual inbox scoped to its aliased address
- Read-only access (messages are never marked as read on the IMAP server)

## Architecture

```
IMAP Server (real inbox)
        │
        ▼
  ┌───────────┐
  │  Submail  │   ← monitors inbox via IMAP (read-only, no mark-as-read)
  │  Server   │
  └─────┬─────┘
        │  REST API
   ┌────┴────┐
   │         │
Agent 1    Agent 2
(bot+agent1) (bot+agent2)
own API key  own API key
```

## Key Concepts

### IMAP Monitoring
- Submail connects to an IMAP server using provided credentials
- It polls or idles on the inbox to fetch new messages
- Messages are **never marked as read** — Submail is purely an observer

### Email Address Routing
- Each monitored address is a plus-alias of the real inbox (e.g. `bot+agent1@example.com`)
- Submail filters messages by the `To` / `Delivered-To` header to route them to the correct virtual inbox
- Multiple aliases can point to the same IMAP account

### Agent Access
- Each agent is assigned an API key mapped to one or more monitored email addresses
- When an agent calls the inbox API, it authenticates with its API key
- Submail returns only messages addressed to that agent's aliased address(es)

## REST API (intended)

| Endpoint | Description |
|----------|-------------|
| `GET /inbox` | List messages in the agent's virtual inbox |
| `GET /inbox/:id` | Get a specific message |

Authentication is via API key (header or query param — TBD).

## Configuration (intended)

The server is expected to be configured with:
- IMAP server host, port, and credentials
- A mapping of agent API keys → monitored email address(es)

## Tech Stack

- **Language:** Go
- **Protocol:** IMAP (read-only inbox monitoring)
- **Interface:** HTTP REST API

## Development

> Project is in early/initial stage. No source files yet.

### Prerequisites
- Go (see `.tool-versions` or `go.mod` for version)

### Running
```bash
go run .
```

### Testing
```bash
go test ./...
```
