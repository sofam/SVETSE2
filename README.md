# SVETSE2

A MegaHAL Markov-chain chatbot reimplemented in Go, with Slack and Discord support.

Learns from everything said in channels it can see, responds with surprise-maximizing
nonsense when @mentioned. The bot is intentionally stupid — that's the fun part.

## Quick Start

```bash
# Build
go build -o svetse2 .

# Run with Slack
export SVETSE2_SLACK_TOKEN=xoxb-...
export SVETSE2_SLACK_APP_TOKEN=xapp-...
./svetse2

# Run with Discord
export SVETSE2_DISCORD_TOKEN=...
./svetse2

# Docker
docker build -t svetse2 .
docker run -e SVETSE2_SLACK_TOKEN=... -e SVETSE2_SLACK_APP_TOKEN=... svetse2
```

## Configuration

All via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SVETSE2_SLACK_TOKEN` | Slack bot token | - |
| `SVETSE2_SLACK_APP_TOKEN` | Slack app-level token (Socket Mode) | - |
| `SVETSE2_DISCORD_TOKEN` | Discord bot token | - |
| `SVETSE2_SLACK_CHANNELS` | Comma-separated channel allowlist | (all) |
| `SVETSE2_DISCORD_CHANNELS` | Comma-separated channel allowlist | (all) |
| `SVETSE2_BRAIN_PATH` | Brain file path | `./brain.bin` |
| `SVETSE2_SAVE_INTERVAL` | Auto-save interval | `5m` |
| `SVETSE2_CHAOS` | Combined chaos dial | `1.0` |
| `SVETSE2_TEMPERATURE` | Random walk temperature | `1.0` |
| `SVETSE2_SURPRISE_BIAS` | Surprise scoring exponent | `1.0` |
| `SVETSE2_REPLY_TIMEOUT` | Reply generation duration | `2s` |
| `SVETSE2_BAN_FILE` | Banned keywords file | `./megahal.ban` |
| `SVETSE2_AUX_FILE` | Auxiliary keywords file | `./megahal.aux` |
| `SVETSE2_SWP_FILE` | Word swap pairs file | `./megahal.swp` |

## Chat Commands

When @mentioning the bot, you can use `!KEY=VALUE` overrides and special commands:

```
@bot tell me about cats                     — get a reply
@bot tell me about cats !CHAOS=2.0          — extra unhinged reply
@bot !HELP                                  — show usage guide
@bot !TRAIN=wiki:Gollum                     — train from English Wikipedia
@bot !TRAIN=wiki:sv:Prinsesstårta           — train from Swedish Wikipedia
@bot !TRAIN=wiki:random                     — train from a random English article
@bot !TRAIN=wiki:sv:random                  — train from a random Swedish article
@bot !TRAIN=https://en.wikipedia.org/wiki/Cat  — train from a Wikipedia URL
```

### Generation Overrides

| Override | Description | Default | Max |
|----------|-------------|---------|-----|
| `!CHAOS=X` | Combined chaos dial (sets temperature + surprise bias) | `1.0` | - |
| `!TEMPERATURE=X` | Random walk temperature | `1.0` | - |
| `!SURPRISE_BIAS=X` | Surprise scoring exponent | `1.0` | - |
| `!TIMEOUT=Xs` | Reply generation time | `2s` | `30s` |

Higher `CHAOS` = wilder, more unhinged replies. Individual knobs override `CHAOS` when both are specified.

### Training

| Command | Description |
|---------|-------------|
| `!TRAIN=wiki:Article` | Train from English Wikipedia article |
| `!TRAIN=wiki:sv:Article` | Train from Swedish (or any language) Wikipedia |
| `!TRAIN=wiki:random` | Train from a random English Wikipedia article |
| `!TRAIN=wiki:sv:random` | Train from a random article in any language |
| `!TRAIN=<wikipedia URL>` | Train from any Wikipedia URL (language auto-detected) |
| `!HELP` | Show available commands |

## CLI Training

Train the brain offline from text files or Wikipedia:

```bash
# Train from text files (one sentence per line)
svetse2 train corpus.txt

# Train from Wikipedia
svetse2 train wiki:Gollum wiki:sv:Sverige

# Train from random articles
svetse2 train wiki:random wiki:sv:random

# Train from URLs
svetse2 train "https://en.wikipedia.org/wiki/MegaHAL"

# Mix sources
svetse2 train corpus.txt wiki:Gollum wiki:random

# Custom brain path
SVETSE2_BRAIN_PATH=./my.brain svetse2 train wiki:Cat
```

## How It Works

SVETSE2 is a reimplementation of [MegaHAL](https://en.wikipedia.org/wiki/MegaHAL) — a Markov-chain chatbot created by Jason Hutchens in 1998.

- **Order-5 Markov chains** with forward and backward trees
- **Surprise-maximizing reply selection**: generates many candidates in a timed loop, scores each by how surprising the input keywords are in the generated context, picks the most surprising one
- **Unicode-aware tokenization**: handles emoji, CJK, accented characters correctly
- **Brain persistence**: custom binary format (SVETSE2v1) with uint32 fields and atomic saves — no more corruption at 100MB like the original C implementation
- **Single goroutine model**: all state owned by one goroutine, no locks needed

## Slack App Setup

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps)
2. Enable **Socket Mode** (generates the app-level token `xapp-...`)
3. Under **Event Subscriptions** → **Subscribe to bot events**, add:
   - `message.channels`
   - `message.groups`
   - `app_mention`
4. Under **OAuth & Permissions**, add bot scopes:
   - `chat:write`
   - `channels:history`
   - `groups:history`
   - `channels:read`
5. Install the app to your workspace
6. Invite the bot to channels: `/invite @botname`
