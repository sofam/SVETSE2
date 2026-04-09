# Channel Backfill

## Overview

Train the brain from a channel's entire message history. Works on both Slack and Discord. Fetching runs in a background goroutine so the bot stays fully responsive during the process.

## Commands

```
@bot !BACKFILL              — backfill the current channel
@bot !BACKFILL=catch-up     — only fetch messages since last backfill
```

## Architecture

### Why background goroutine + one-shot train

Fetching channel history is slow (rate-limited API calls over minutes). Training from in-memory text is fast (seconds). So we separate the two:

1. **Background goroutine** fetches all messages, accumulates them in `[]string`
2. When done, sends a `BulkLearnRequest` to the model goroutine
3. **Model goroutine** trains on the entire corpus in one shot, saves brain, updates state

The bot remains fully responsive to mentions and learning during the fetch phase.

### Request Types

```go
type BulkLearnRequest struct {
    Texts     []string
    ChannelID string
    Platform  string  // "slack" or "discord"
    ReplyCh   chan string
}
```

### Message Fetcher Interface

Both platforms implement the same interface, with rate limit sleeps baked in:

```go
type MessageFetcher interface {
    // FetchPage returns a batch of message texts and a cursor for the next page.
    // Returns empty cursor when there are no more pages.
    FetchPage(channelID string, cursor string) (messages []string, nextCursor string, err error)
}
```

## State Tracking

A JSON sidecar file (`brain.state.json`) next to the brain file tracks backfill progress:

```json
{
  "backfill": {
    "slack:C01QTGBL9JR": {
      "last_ts": "1775609573.830919",
      "count": 12345,
      "completed_at": "2026-04-09T10:30:00Z"
    },
    "discord:123456789012345678": {
      "last_id": "1234567890123456789",
      "count": 8765,
      "completed_at": "2026-04-09T11:00:00Z"
    }
  }
}
```

- `last_ts` / `last_id`: platform-specific marker of the most recent message processed
- `count`: total messages learned from this channel
- `completed_at`: when the backfill finished

## Flow

### Initial Backfill

```
User: @bot !BACKFILL
  │
  ▼
Platform adapter:
  1. Check state file — already backfilled?
     → Yes: reply "Already backfilled (12345 messages on 2026-04-09). Use !BACKFILL=catch-up to fetch new messages."
     → No: continue
  2. Check in-progress map — already running for this channel?
     → Yes: reply "Backfill already in progress."
     → No: continue
  3. Mark channel as in-progress
  4. Reply "Backfill started, fetching history..."
  5. Spawn background goroutine
  │
  ▼
Background goroutine:
  1. Paginate through channel history (newest to oldest)
  2. For each message:
     - Skip bot messages
     - Skip messages that mention the bot
     - Skip blockquoted messages (> prefix)
     - Collect cleaned text into []string
  3. Post progress to channel every 1000 messages:
     "Backfill in progress... 3000 messages fetched"
  4. When pagination complete, send BulkLearnRequest to model goroutine
  │
  ▼
Model goroutine:
  1. Receive BulkLearnRequest
  2. Loop through texts, call learn() on each
  3. Save brain
  4. Update state file with last message marker + count
  5. Send completion message back via ReplyCh
  │
  ▼
Background goroutine:
  1. Receive completion message
  2. Post to channel: "Backfill complete: learned 12345 messages"
  3. Remove channel from in-progress map
```

### Catch-up Backfill

Same flow, but pagination starts from the stored `last_ts`/`last_id` instead of the newest message. Fetches only messages newer than the last backfill.

After completion, updates the state file marker to the newest message seen.

## Rate Limits

### Discord

- `GET /channels/{id}/messages`: max 100 messages per request
- Rate limit: 50 req/s per route (but be conservative)
- **Strategy**: 500ms sleep between requests
- 50K messages = 500 requests = ~4 minutes

### Slack

- `conversations.history`: max 200 messages per request
- Rate limit: Tier 3 = ~50 req/min
- **Strategy**: 1.2s sleep between requests
- 50K messages = 250 requests = ~5 minutes

## Concurrency Guards

- A `map[string]bool` (protected by mutex) tracks which channels have an in-progress backfill
- Prevents duplicate backfills of the same channel
- Cleared on completion or error

```go
var (
    backfillMu     sync.Mutex
    backfillActive = make(map[string]bool) // key: "platform:channelID"
)
```

## Message Filtering

Same rules as live learning:
- Skip bot messages
- Skip messages that @mention the bot (prevents metric skewing)
- Skip blockquoted messages (`>` / `&gt;` prefix in Slack)
- Clean platform formatting before learning (strip mentions, URLs, etc.)

## Error Handling

- If fetching fails mid-way, post error to channel, clear in-progress flag
- Do NOT update state file on failure — next `!BACKFILL` will retry from scratch
- If `BulkLearnRequest` fails (unlikely — it's just calling learn()), report error

## Files to Create/Modify

- Create: `backfill.go` — BackfillRequest, BulkLearnRequest, state file read/write, concurrency guards
- Create: `backfill_slack.go` — Slack MessageFetcher implementation
- Create: `backfill_discord.go` — Discord MessageFetcher implementation
- Modify: `main.go` — add BulkLearnRequest to model goroutine select loop, add backfillCh
- Modify: `platform_slack.go` — handle `!BACKFILL` in AppMentionEvent, spawn goroutine
- Modify: `platform_discord.go` — handle `!BACKFILL` in MessageCreate handler, spawn goroutine
- Modify: `megahal.go` — add `BACKFILL` to parseOverrides regex
