# Protobuf Brain Format

## Overview

Replace the hand-rolled SVETSE2v1 binary brain format with Protocol Buffers for schema evolution, backward compatibility, and better tooling. The in-memory model stays the same — only the serialization layer changes.

## Motivation

The current binary format works but is rigid:
- Adding a field (e.g. backfill state, training metadata) requires a new cookie version and migration code
- No tooling to inspect brain files — completely opaque
- No schema documentation beyond reading the save/load Go code

With protobuf:
- Adding fields is backward compatible by default
- `protoc --decode` can inspect brain files from the command line
- Schema is self-documenting (the `.proto` file)
- Well-tested serialization — no hand-rolled read/write bugs

## Schema

```protobuf
syntax = "proto3";

package svetse2;

option go_package = "github.com/sofam/SVETSE2";

message Brain {
  uint32 order = 1;
  Node forward = 2;
  Node backward = 3;
  repeated string dictionary = 4;
  // Future fields added here are automatically backward compatible:
  // map<string, ChannelState> backfill = 5;
  // repeated TrainingSource sources = 6;
  // BrainMetadata metadata = 7;
}

message Node {
  uint32 symbol = 1;
  uint32 usage = 2;
  uint32 count = 3;
  repeated Node children = 4;
}

// Reserved for future use with backfill feature
// message ChannelState {
//   string last_marker = 1;  // message ID or timestamp
//   int64 count = 2;
//   string completed_at = 3; // RFC3339
// }
```

## File Format

```
[4 bytes] magic: "SV2P" (distinguishes from old "SVETSE2v1" format)
[rest]    proto-encoded Brain message
```

The 4-byte magic prefix lets `loadBrain` detect which format it's reading and support both during a transition period.

## Migration Strategy

### Phase 1: Dual Read

- `loadBrain` checks the first bytes:
  - Starts with `"SVETSE2v1"` → load with old binary reader
  - Starts with `"SV2P"` → load with protobuf
- `saveBrain` always writes protobuf format
- First save after loading an old brain silently migrates it

### Phase 2: Remove Old Reader

After a reasonable period (or immediately if no one has old brains they care about), remove the old binary reader code.

## Size Comparison

For a brain with ~4000 dictionary entries and typical tree depth:

| Format | Approximate Size | Notes |
|--------|-----------------|-------|
| SVETSE2v1 (current) | Baseline | Fixed-size uint32 fields, no overhead |
| Protobuf | ~10-20% larger | Varint encoding + field tags. Small symbols/counts use fewer bytes (varint), but field tags add overhead. Roughly a wash. |

For brains that fit in memory (which they all do), the size difference is irrelevant.

## Atomic Save

Same strategy as current: write to temp file, `fsync`, rename over target. No change needed.

## Dependencies

- `google.golang.org/protobuf` — Go protobuf runtime
- `protoc` + `protoc-gen-go` — code generation (build-time only)

Add to the project:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

## Files to Create/Modify

- Create: `brain.proto` — schema definition
- Create: `brain_pb.go` — generated code (from `protoc`)
- Modify: `brain.go` — replace saveTree/loadTree/saveDictionary/loadDictionary with proto marshal/unmarshal, add dual-read support
- Modify: `brain_test.go` — add migration test (load old format, save new, reload)
- Modify: `Makefile` or `go generate` directive for proto compilation

## Inspecting Brain Files

After migration, brain files can be inspected:

```bash
# Skip the 4-byte magic, decode the rest
tail -c +5 brain.bin | protoc --decode=svetse2.Brain brain.proto
```

Or write a small `svetse2 inspect` subcommand that prints brain stats (dictionary size, tree depth, top words, etc.).
