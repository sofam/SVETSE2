# SVETSE2

MegaHAL Markov-chain chatbot in Go. Single binary, flat `package main`. Slack + Discord.

## Build & Test

```bash
make          # build
make test     # run tests
make vet      # go vet
make linux    # cross-compile for linux/amd64
make docker   # docker build
make clean    # remove binaries
```

## Architecture

Single goroutine owns the model — all learn/reply/train requests go through channels in `main.go`. Platform adapters are thin event routers. No locks needed.

```
main.go              — config, model goroutine, signal handling, request types
megahal.go           — Model, Node, dictionary, tokenizer, learn, reply generation, overrides
brain.go             — save/load binary format (SVETSE2v1), atomic writes
cmd_train.go         — "train" subcommand, Wikipedia fetching, !TRAIN handler
platform_slack.go    — Slack Socket Mode adapter (AppMentionEvent for replies, MessageEvent for learning)
platform_discord.go  — Discord gateway adapter
```

## Key Design Decisions

- **Dictionary[0] = ""** is the boundary/end-of-sentence symbol. `findWord` returns 0 for unknown words, which doubles as the boundary marker.
- **All text is uppercased** in the tokenizer (`makeWords`). Output is lowercased then sentence-capitalized in `makeOutput`.
- **Children sorted by Symbol** for binary search in `searchNode`.
- **AppMentionEvent vs MessageEvent**: Slack sends both for @mentions. `MessageEvent` only handles learning, `AppMentionEvent` handles all replies/commands to avoid double-processing.
- **Messages mentioning the bot are never learned** to prevent metric skewing.
- **Blockquoted messages (> prefix) are skipped** for learning.
- **Reply generation** uses a timed loop generating candidates and picking the highest-surprise one. Temperature modifies the random walk, surprise bias modifies the scoring.

## Conventions

- Flat package structure — everything in `package main`, no sub-packages
- No interfaces or abstractions beyond what's needed
- Tests in `*_test.go` alongside source files
- Commits use conventional commit style (`feat:`, `fix:`, `test:`, `docs:`, `chore:`)

## Unimplemented Specs

- `docs/superpowers/specs/unimplemented/backfill.md` — channel history backfill
- `docs/superpowers/specs/unimplemented/protobuf-brain.md` — protobuf brain format migration
