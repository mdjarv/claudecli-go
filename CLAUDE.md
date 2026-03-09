# Commands

- Build: `go build ./...`
- Test: `go test ./... -count=1`
- Test with race detector: `go test -race ./... -count=1`
- Vet: `go vet ./...`
- Benchmarks: `go test -bench=. -benchmem ./...`
- Integration tests (requires real CLI + API key): `go test -tags=integration ./... -count=1`

# Gotchas

- Verify CLI flags exist (`claude --help | grep`) before adding new options to `buildArgs()` — the CLI has no formal spec and flags change between versions.
- Update README.md (options table, architecture, known limitations) before considering a task complete.
