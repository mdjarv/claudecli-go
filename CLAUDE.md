# Commands

- Build: `go build ./...`
- Test: `go test ./... -count=1`
- Test with race detector: `go test -race ./... -count=1`
- Vet: `go vet ./...`
- Benchmarks: `go test -bench=. -benchmem ./...`
- Integration tests (requires real CLI + API key): `go test -tags=integration ./... -count=1`

# Investigating CLI behavior

The CLI has no formal spec. When investigating what events/fields the CLI actually emits, use `cmd/capture` to record raw traffic:

```bash
# Capture raw JSONL from a live CLI session:
go run ./cmd/capture -prompt "your prompt here" -out tmp

# Replay and analyze a captured file:
go run ./cmd/capture -analyze tmp/raw-stdout.jsonl
```

Output: `tmp/raw-stdout.jsonl` (raw JSONL), `tmp/raw-stderr.log` (stderr). The tool also pipes through `ParseEvents` and prints a summary of event types seen, highlighting any `UnknownEvent` instances.

Flags: `-prompt` (default triggers Agent tool), `-out` (output dir, default `tmp`), `-timeout` (default 2m), `-analyze` (replay mode).

# Gotchas

- Verify CLI flags exist (`claude --help | grep`) before adding new options to `buildArgs()` — the CLI has no formal spec and flags change between versions.
- Update README.md (options table, architecture, known limitations) before considering a task complete.
