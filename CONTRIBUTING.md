# Contributing to gdrive-bisync

The README is intentionally written for end users. Development, testing, architecture, and release information belongs here.

## Requirements

- Go 1.25 or newer
- A test Google Drive setup for manual integration testing
- `golangci-lint` for the complete static-analysis pass

## Development commands

| Task | Command |
|---|---|
| Run locally | `make dev` |
| Unit and integration tests | `make test` |
| Race detector | `go test -race ./...` |
| Static analysis | `golangci-lint run ./...` |
| Go vet | `make vet` |
| Linux build | `make linux` |
| Windows build | `make windows` |
| Release archives | `make release` |

## Architecture

```text
cmd/gdrive-bisync       CLI, daemon lifecycle, and runtime controls
internal/api            Google Drive and OAuth integration
internal/appstate       Lock, status, event journal, and control files
internal/core           Planning, reconciliation, watcher, and execution
internal/store          bbolt state and database backups
internal/services       Logging, notifications, scanning, and systemd
internal/tui            Bubble Tea observability dashboard
internal/utils          Trash, restore, and path utilities
```

The daemon is the only writer of synchronization state. The TUI reads atomic runtime state and the bounded event journal, then sends one-shot control requests through runtime marker files.

## Verification before submitting a change

```bash
gofmt -w $(git ls-files '*.go')
golangci-lint run ./...
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
GOOS=windows GOARCH=amd64 go build ./cmd/gdrive-bisync
go build ./cmd/gdrive-bisync
```

Changes to deletion planning, trash restoration, persistence, locking, or path handling require targeted regression tests because those areas protect user data.

## Documentation responsibilities

- Keep `README.md` focused on installation and product usage.
- Put contributor workflows and internal architecture in this file.
- Document user-visible flags and configuration changes in the README.
- Avoid examples containing real filenames, tokens, folder IDs, or private paths.
