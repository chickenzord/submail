---
name: submail-client
description: Read emails from a Submail virtual inbox using the submail CLI. Use when an agent needs to check, list, or retrieve emails delivered to its inbox address.
metadata:
  {
    "author": "akhy",
    "version": "1.0.0",
    "openclaw":
      {
        "emoji": "📬",
        "homepage": "https://github.com/chickenzord/submail",
        "keywords": ["email", "inbox", "imap", "submail", "cli", "agent", "mail"],
        "requires": { "bins": ["submail"] },
        "install": [],
      },
  }
---

# Submail Client Skill

Submail is a virtual inbox router. Each agent has a dedicated email address and
API token. This skill covers everything needed to configure and use the
`submail` CLI to access that inbox.

---

## Prerequisites

- `submail` binary is on `$PATH`
- You know your **server URL**, **agent token**, and (optionally) your **profile name**

---

## Step 1 — Configure a profile (one-time setup)

Profiles store the server URL and token so you never pass them as flags.
Profile files live at `~/.config/submail/profiles/<name>.yaml`.

```bash
submail profile set <profile-name> \
  --url <server-url> \
  --token <your-agent-token>
```

Example:
```bash
submail profile set hawkeye \
  --url http://localhost:8080 \
  --token hawkeye-secret-token
```

### Validate the profile was saved correctly

```bash
submail profile show <profile-name> --json
```

Expected output (token is always masked):
```json
{"name":"hawkeye","token_set":true,"url":"http://localhost:8080"}
```

Check: `token_set` must be `true` and `url` must be non-empty. If either is
wrong, re-run `profile set` with the correct values.

### Set the active profile for this agent session

```bash
export SUBMAIL_PROFILE=<profile-name>
```

After this, all `submail inbox` commands use the profile automatically — no
`--url` or `--token` flags needed.

---

## Step 2 — Verify connectivity

Run a quick sanity check before doing real work:

```bash
submail inbox list --json --limit 1
```

| Exit code | Meaning | Fix |
|-----------|---------|-----|
| `0` | Connected and authenticated | — |
| `4` | Token rejected | Re-run `profile set` with the correct token |
| `1` | Server unreachable | Check `url` in the profile; confirm server is running |

A successful response looks like:
```json
{"mails":[...],"total":5,"limit":1,"offset":0}
```

If `total` is `0` the inbox is empty — that is not an error.

---

## Available tasks

### List messages

```bash
submail inbox list --json
```

Response schema:
```json
{
  "mails": [
    {
      "id":          "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "message_id":  "<envelope-message-id@sender.example>",
      "subject":     "Subject line",
      "from":        "sender@example.com",
      "to":          "bot+agentname@example.com",
      "received_at": "2026-01-01T00:00:00Z"
    }
  ],
  "total":  42,
  "limit":  50,
  "offset": 0
}
```

Key fields:
- `id` — UUID used to fetch the full message; use this in `inbox get`
- `received_at` — ISO 8601 UTC timestamp
- `total` — total matching messages (ignores `limit`/`offset`)
- `mails` may be an empty array `[]` — that is not an error

#### Pagination

```bash
# First page
submail inbox list --json --limit 20 --offset 0

# Second page
submail inbox list --json --limit 20 --offset 20
```

Iterate until `offset + len(mails) >= total`.

#### Get only IDs (pipe-friendly)

```bash
submail inbox list --quiet
```

Outputs one UUID per line. Useful for looping or piping.

---

### Get a single message

```bash
submail inbox get --json <id>
```

`<id>` must be a lowercase UUID (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`).
Obtain it from the `id` field in `inbox list`.

Response schema (same as list items, plus body fields):
```json
{
  "id":          "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "message_id":  "<envelope-message-id@sender.example>",
  "subject":     "Subject line",
  "from":        "sender@example.com",
  "to":          "bot+agentname@example.com",
  "received_at": "2026-01-01T00:00:00Z",
  "text_body":   "Plain-text content of the email.",
  "html_body":   "<html>...</html>"
}
```

- `text_body` — prefer this for content parsing; may be empty for HTML-only emails
- `html_body` — raw HTML; present when the sender included an HTML part
- Both fields are omitted (not `null`) when absent

---

## Exit codes

| Code | Meaning | Retryable |
|------|---------|-----------|
| `0` | Success | — |
| `2` | Bad input (e.g. malformed UUID) | No — fix the input |
| `3` | Message not found | No |
| `4` | Unauthorized — wrong token | No — fix the profile |
| `1` | Server / network error | Yes |

Always check `$?` after each command. On non-zero exit (JSON mode), stdout
contains a structured error:

```json
{
  "error":     "not_found",
  "message":   "mail not found",
  "input":     {"id": "00000000-0000-0000-0000-000000000000"},
  "retryable": false
}
```

Use `retryable` to decide whether to retry or abort.

---

## Common workflows

### Read the most recent message

```bash
ID=$(submail inbox list --quiet --limit 1)
submail inbox get --json "$ID"
```

### Process all messages newest-first

```bash
LIMIT=20
OFFSET=0

while true; do
  PAGE=$(submail inbox list --json --limit $LIMIT --offset $OFFSET)
  TOTAL=$(echo "$PAGE" | jq '.total')
  IDS=$(echo "$PAGE"  | jq -r '.mails[].id')

  for ID in $IDS; do
    submail inbox get --json "$ID" | jq '.text_body'
  done

  OFFSET=$((OFFSET + LIMIT))
  [ $OFFSET -ge $TOTAL ] && break
done
```

### Check for new mail since a known timestamp

`inbox list` does not have a `since` filter — fetch the list and filter
client-side by `received_at`:

```bash
submail inbox list --json --limit 50 | \
  jq '[.mails[] | select(.received_at > "2026-01-01T00:00:00Z")]'
```

---

## Profile reference

| Command | Purpose |
|---------|---------|
| `submail profile set <name> --url URL --token TOKEN` | Create or update a profile |
| `submail profile show <name> --json` | Verify a profile (token masked) |
| `submail profile list --json` | List all configured profiles |
| `submail profile delete <name>` | Remove a profile |

Profile files are stored at `~/.config/submail/profiles/<name>.yaml` with
mode `0600` (readable only by the current OS user).

---

## Credential precedence (most specific wins)

1. `--url` / `--token` flags
2. `SUBMAIL_URL` / `SUBMAIL_TOKEN` environment variables
3. `--profile` flag or `SUBMAIL_PROFILE` environment variable
4. `~/.config/submail/profiles/default.yaml` (silent fallback)
