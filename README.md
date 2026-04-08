# SVETSE2

A MegaHAL Markov-chain chatbot reimplemented in Go, with Slack and Discord support.

Learns from everything said in channels it can see, responds with surprise-maximizing
nonsense when @mentioned.

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

## Per-Message Overrides

When @mentioning the bot, add `!KEY=VALUE` to override settings for that reply:

```
@bot tell me about cats !CHAOS=2.0 !TIMEOUT=5s
```

| Override | Description |
|----------|-------------|
| `!CHAOS=X` | Combined chaos dial |
| `!TEMPERATURE=X` | Random walk temperature |
| `!SURPRISE_BIAS=X` | Surprise scoring exponent |
| `!TIMEOUT=Xs` | Reply generation time (max 30s) |
| `!HELP` | Show usage guide |
