# Contributing to gdrive-bisync

Thanks for helping improve gdrive-bisync. Bug fixes, tests, documentation, and focused features are welcome.

## Before you start

- Search existing issues and pull requests first.
- Open an issue before starting a large feature or major redesign.
- Never include OAuth credentials, tokens, Drive IDs, or private filenames in code, tests, screenshots, or logs.
- Test against a dedicated temporary Drive folder—not your main Drive.

## Development workflow

Pull requests should target the `dev` branch. Do not open feature pull requests directly against `main`.

### 1. Fork and clone

```bash
git clone https://github.com/YOUR_USERNAME/gdrive-bisync.git
cd gdrive-bisync
git remote add upstream https://github.com/AzPepoze/gdrive-bisync.git
```

### 2. Start from the latest `dev`

```bash
git fetch upstream
git switch -c fix/short-description upstream/dev
```

Use a clear branch name:

```text
fix/prevent-duplicate-upload
feat/sync-progress
docs/setup-guide
test/trash-restore
```

### 3. Make a focused change

- Keep the pull request limited to one problem.
- Add regression tests for bug fixes.
- Preserve unrelated code and behavior.
- Update the README when flags, configuration, or user behavior changes.
- Put contributor and internal details in this file, not the README.

### 4. Verify your work

Go 1.25 or newer is required.

```bash
gofmt -w $(git ls-files '*.go')
golangci-lint run ./...
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/gdrive-bisync
GOOS=windows GOARCH=amd64 go build ./cmd/gdrive-bisync
```

Changes involving deletion, trash, path handling, locking, downloads, or sync-state persistence need targeted tests because those areas protect user data.

### 5. Commit and push

Use a short commit message that explains the result:

```text
fix: prevent duplicate watcher uploads
feat: show retry progress in TUI
docs: clarify OAuth setup
```

Then push your branch:

```bash
git push -u origin fix/short-description
```

### 6. Open a pull request

Open the pull request with:

- Base repository: `AzPepoze/gdrive-bisync`
- Base branch: `dev`
- Compare branch: your feature branch

Include:

- What problem it fixes
- What changed
- How you tested it
- Any user-visible behavior or configuration changes
- Screenshots for TUI changes, with private paths hidden

## Pull request checklist

- [ ] The PR targets `dev`.
- [ ] The change is focused and does not include unrelated formatting.
- [ ] Tests cover new behavior or the reported bug.
- [ ] Unit tests and the race detector pass.
- [ ] `golangci-lint` and `go vet` pass.
- [ ] Linux and Windows builds succeed.
- [ ] User-facing changes are documented.
- [ ] No private data, credentials, or tokens are included.

## Project layout

| Path | Responsibility |
|---|---|
| `cmd/gdrive-bisync` | CLI and daemon lifecycle |
| `internal/api` | Google Drive and OAuth |
| `internal/appstate` | Runtime status, events, locks, and controls |
| `internal/core` | Sync planning and execution |
| `internal/store` | Sync database and backups |
| `internal/services` | Logging, notifications, scanning, and systemd |
| `internal/tui` | Bubble Tea dashboard |
| `internal/utils` | Trash, restore, and path helpers |

## Safety expectations

gdrive-bisync handles user files. Prefer stopping safely over guessing.

- Do not bypass deletion limits implicitly.
- Do not permanently delete files when trash is available.
- Keep writes atomic where possible.
- Validate that paths remain inside the sync root.
- Return and test errors from destructive operations.
- Avoid logging private filenames in tests or examples.

## Review

Maintainers may request smaller commits, more tests, or a safer design. Address review comments on the same branch; the pull request updates automatically when you push new commits.
