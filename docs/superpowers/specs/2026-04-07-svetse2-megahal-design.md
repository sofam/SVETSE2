# SVETSE2: MegaHAL Reimplementation in Go

## Overview

A pure Go reimplementation of the MegaHAL Markov-chain chatbot, replacing the legacy C binary + C# Slack wrapper (slackseNET). The bot connects to both Slack and Discord, learns from all channel activity, and responds with surprise-maximizing nonsense when @mentioned.

The original C implementation suffers from uint16 field overflow at ~100MB brain size, ISO-8859-1 encoding (no unicode/emoji), and depends on a precompiled Linux binary. This reimplementation fixes all of these while faithfully reproducing the core algorithm.

## Architecture

Single Go binary, flat package structure. All source in `package main`.

```
svetse2/
├── main.go                # entry, config, wiring, signal handling
├── megahal.go             # Model, Node, Dictionary, learn, reply, tokenize
├── megahal_test.go        # unit tests
├── brain.go               # save/load binary format, atomic write
├── brain_test.go
├── platform_slack.go      # Slack Socket Mode adapter
├── platform_discord.go    # Discord gateway adapter
├── Dockerfile             # multi-stage Go build
├── go.mod
├── megahal.ban            # default ban list
└── README.md
```

## MegaHAL Engine

### Data Structures

**Dictionary:**
- `[]string` for entries indexed by symbol ID. Index 0 is reserved as the boundary symbol.
- `map[string]uint32` for reverse lookup (word to symbol).
- All strings are native Go UTF-8. No length limits.

**Tree Node:**
```go
type Node struct {
    Symbol   uint32
    Usage    uint32
    Count    uint32
    Children []*Node // sorted by Symbol for binary search
}
```

All fields uint32. The old C code used uint16 for Symbol (max 65535 unique words), Count (frequency capped at 65535), and branch/children count (max 65535 children per node). These overflows are the root cause of the ~100MB brain corruption — once the dictionary exceeds 65535 words or a node exceeds 65535 children, indices wrap around and the tree becomes nonsensical.

**Model:**
```go
type Model struct {
    Order      int
    Forward    *Node
    Backward   *Node
    Dictionary []string
    DictMap    map[string]uint32
    Context    []*Node // working context during traversal
}
```

### Tokenization

Split input into alternating word/separator tokens. Boundary detection uses `unicode.IsLetter` and `unicode.IsDigit` instead of the original ASCII `isalpha`. This means emoji, CJK characters, and accented characters are handled correctly as tokens rather than being mangled on byte boundaries.

Example: `"Hello, world! 🎉"` → `["Hello", ", ", "world", "! ", "🎉"]`

Punctuation is preserved as separate tokens — it carries semantic weight in the Markov chain.

### Learning

For each input sentence:
1. Tokenize into word/separator tokens.
2. Map each token to a symbol ID (add to dictionary if new).
3. Walk the token list forward through the forward tree: at each step, find-or-create the child node for the current symbol, increment its count and the parent's usage.
4. Walk the token list backward through the backward tree with the same logic.
5. Boundary symbol (0) marks sentence start and end.

### Reply Generation

1. Extract keywords from input: words that exist in the dictionary, excluding banned words (from `megahal.ban`) and auxiliary words (from `megahal.aux`).
2. Run a timed loop (default 2 seconds, configurable):
   a. Pick a random keyword as a seed.
   b. Generate a candidate reply by walking forward from the seed through the forward tree (probabilistic selection weighted by count/usage, modified by temperature), then backward through the backward tree to complete the sentence.
   c. Score the candidate with `evaluate_reply`: compute surprise (negative log-probability) of the input keywords appearing in the candidate, measured against both forward and backward trees. Apply surprise bias exponent.
   d. Keep the candidate with the highest surprise score that is dissimilar to the input.
3. Return the best reply found.

### Surprise and Chaos Configuration

The bot's character comes from surprise-maximizing selection. Three knobs control this:

**Temperature** (default 1.0): Applied during the random walk that generates candidates. Temperature > 1.0 flattens the probability distribution, making unlikely word transitions more probable. The random walks become wilder, producing more diverse and unhinged candidates. Temperature < 1.0 produces more conservative, predictable output.

**Surprise Bias** (default 1.0): An exponent applied to the surprise score. Values > 1.0 amplify differences between candidates, aggressively preferring the truly deranged replies. Values < 1.0 make the selection more uniform.

**Reply Timeout** (default 2s): How long the generation loop runs. More time means more candidates evaluated, increasing the chance of finding a high-surprise reply.

**Chaos** (default 1.0): A convenience dial. `CHAOS=X` sets temperature to `X` and surprise bias to `X` (both scale together — higher chaos means wilder walks AND more aggressive surprise selection). Individual knobs override the chaos dial when both are specified.

All four are configurable via environment variables and overridable per-message.

### Keyword Support Files

All optional. Missing files mean empty lists.

- `megahal.ban`: Words to never use as keywords (common words: the, a, is, etc.)
- `megahal.aux`: Words to use as keywords only if no better ones are found.
- `megahal.swp`: Word swap pairs for reply generation (e.g., "my" ↔ "your").

## Brain Persistence

### Binary Format

All multi-byte integers are little-endian. Header: `"SVETSE2v1"` (9 bytes), followed by:

```
Order:      uint8
Forward:    tree (recursive)
Backward:   tree (recursive)
Dictionary: uint32 entry_count
             for each entry: uint32 byte_length, []byte utf8_string
```

Tree node on disk (recursive):
```
Symbol:       uint32
Usage:        uint32
Count:        uint32
NumChildren:  uint32
[children...] (recursive, NumChildren times)
```

### Save Strategy

- Write to a temporary file in the same directory, then atomic rename over the target path. A crash mid-save cannot corrupt the brain.
- Save interval configurable via `SVETSE2_SAVE_INTERVAL` (default 5 minutes).
- Save on graceful shutdown (SIGINT/SIGTERM).
- Saves are handled by the model goroutine — no concurrency issues.

## Concurrency Model

A single goroutine owns the model. All learn and reply requests are sent to it via a Go channel.

```go
type LearnRequest struct {
    Text string
}

type ReplyRequest struct {
    Text      string
    Overrides map[string]string
    ReplyCh   chan string
}
```

Learning is fire-and-forget. Reply requests include a response channel. The model goroutine processes requests sequentially — serialization is fine since reply generation only takes a few seconds and the bot is not handling thousands of concurrent requests.

## Platform Adapters

### Common Behavior

Both adapters follow the same rules:

- **Learn from all messages** across all visible channels, as long as the message does NOT contain an @mention of the bot. Messages mentioning the bot are never learned from, to prevent metric skewing (e.g., spamming `@bot penis` 500 times).
- **Respond only when @mentioned**, and only in allowed channels (or everywhere if no allowlist configured).
- **Replies are top-level messages** in the same channel as the mention. Never threaded.
- **Parse `!KEY=VALUE` overrides** from the mention message. Strip them and the @mention from the text before passing to the model.
- **`!HELP`** in a mention message skips reply generation and returns a usage guide showing available overrides and current defaults.

### Slack Adapter

- Library: `github.com/slack-go/slack` with Socket Mode.
- Requires both a bot token (`SVETSE2_SLACK_TOKEN`) and an app-level token (`SVETSE2_SLACK_APP_TOKEN`) for Socket Mode.
- Strips Slack formatting (`<@USERID>`, `<#CHANNEL|name>`, URL wrappers) before learning or replying.
- Channel allowlist: `SVETSE2_SLACK_CHANNELS` (comma-separated channel names or IDs). Empty means respond wherever mentioned.

### Discord Adapter

- Library: `github.com/bwmarrin/discordgo`.
- Requires a bot token (`SVETSE2_DISCORD_TOKEN`).
- Strips Discord mention formatting before learning or replying.
- Channel allowlist: `SVETSE2_DISCORD_CHANNELS` (comma-separated channel names or IDs). Empty means respond wherever mentioned.

## Per-Message Overrides

When @mentioning the bot, users can include `!KEY=VALUE` pairs anywhere in the message to override generation settings for that single reply.

Example: `@bot gollum is a fucking asshole !CHAOS=1.5 !TIMEOUT=5s`

Parsed overrides are stripped from the input text. Available keys:

| Key | Description | Default | Max |
|-----|-------------|---------|-----|
| `!CHAOS` | Combined chaos dial (sets temperature + surprise bias) | 1.0 | - |
| `!TEMPERATURE` | Direct temperature override | 1.0 | - |
| `!SURPRISE_BIAS` | Direct surprise exponent override | 1.0 | - |
| `!TIMEOUT` | Reply generation duration | 2s | 30s |

`!HELP` (no value) shows a usage guide instead of generating a reply.

Individual knobs (`!TEMPERATURE`, `!SURPRISE_BIAS`) override the `!CHAOS` combined dial when both are present.

## Configuration

All via environment variables:

```
# Platform tokens (presence enables the platform)
SVETSE2_SLACK_TOKEN=xoxb-...
SVETSE2_SLACK_APP_TOKEN=xapp-...
SVETSE2_DISCORD_TOKEN=...

# Channel allowlists (empty = respond wherever mentioned)
SVETSE2_SLACK_CHANNELS=#bot-arena,#general
SVETSE2_DISCORD_CHANNELS=bot-arena

# Brain
SVETSE2_BRAIN_PATH=./brain.bin
SVETSE2_SAVE_INTERVAL=5m

# Generation defaults
SVETSE2_CHAOS=1.0
SVETSE2_TEMPERATURE=1.0
SVETSE2_SURPRISE_BIAS=1.0
SVETSE2_REPLY_TIMEOUT=2s

# Support files (optional)
SVETSE2_BAN_FILE=./megahal.ban
SVETSE2_AUX_FILE=./megahal.aux
SVETSE2_SWP_FILE=./megahal.swp
```

If neither Slack nor Discord tokens are set, the binary exits with an error.

## Dockerfile

Multi-stage build:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o svetse2 .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/svetse2 .
COPY megahal.ban megahal.aux megahal.swp ./
ENTRYPOINT ["./svetse2"]
```

## Dependencies

- `github.com/slack-go/slack` — Slack Socket Mode client
- `github.com/bwmarrin/discordgo` — Discord gateway client
- Standard library only for everything else (encoding/binary, math, unicode, os, etc.)
