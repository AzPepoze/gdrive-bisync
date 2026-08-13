# gdrive-bisync

Keep a local folder and Google Drive synchronized in both directions, with a terminal dashboard and safeguards against accidental deletion.

```text
Local folder  ⇄  gdrive-bisync  ⇄  Google Drive
                       │
               Terminal dashboard
```

## What it does

| Feature | What you get |
|---|---|
| Two-way sync | Upload local changes and download Drive changes automatically |
| Live updates | Watch local files and periodically check Google Drive |
| Safe deletion | Preview changes, limit mass deletion, and recover locally deleted items |
| Reliable transfers | Atomic downloads, retries, database backups, and single-process locking |
| Terminal dashboard | View health, progress, recent activity, logs, trash, and safety status |
| Notifications | Receive desktop alerts for important failures and recovery on Linux |

gdrive-bisync works in the background as a systemd user service on Linux. Running `gdrive-bisync` in a terminal opens the Bubble Tea dashboard instead of starting a second sync process.

## Quick start

```bash
# 1. Authenticate after adding credentials.json
gdrive-bisync --setup

# 2. Preview the first synchronization
gdrive-bisync --dry-run

# 3. Install and start the background service
gdrive-bisync --install-service
systemctl --user enable --now gdrive-bisync

# 4. Open the dashboard
gdrive-bisync
```

The default setup synchronizes your entire Google Drive with `~/GoogleDrive`.

## Installation

### Arch Linux

```bash
paru -S gdrive-bisync
```

or:

```bash
yay -S gdrive-bisync
```

### Build from source

Go 1.25 or newer is required.

```bash
git clone https://github.com/AzPepoze/gdrive-bisync.git
cd gdrive-bisync
go build -o gdrive-bisync ./cmd/gdrive-bisync
sudo install -m 0755 gdrive-bisync /usr/local/bin/gdrive-bisync
```

Development and release instructions are in [CONTRIBUTING.md](CONTRIBUTING.md).

## Getting started

### 1. Create Google OAuth credentials

1. Open the [Google Cloud Console](https://console.cloud.google.com/).
2. Create or select a project.
3. Enable the Google Drive API.
4. Open **APIs & Services → Credentials**.
5. Create an **OAuth client ID** with application type **Desktop app**.
6. Download the JSON credentials file.

### 2. Create the configuration directory

Linux:

```bash
mkdir -p ~/.config/gdrive-bisync/config
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME\.config\gdrive-bisync\config"
```

Rename the downloaded OAuth file to `credentials.json` and place it inside that directory.

### 3. Create `config.json`

Place `config.json` beside `credentials.json`:

```json
{
  "LOCAL_SYNC_PATH": "~/GoogleDrive",
  "REMOTE_FOLDER_ID": "root"
}
```

`root` means your entire **My Drive**. To sync only one Drive folder, copy the folder ID from its URL:

```text
https://drive.google.com/drive/folders/YOUR_FOLDER_ID
```

### 4. Authenticate

```bash
gdrive-bisync --setup
```

Your browser opens for Google authorization. The resulting token stays in your local configuration directory.

### 5. Preview before syncing

```bash
gdrive-bisync --dry-run
```

Review the planned uploads, downloads, folder creation, and deletions. A dry run does not change local files, Drive files, or the sync database.

### 6. Run in the background

```bash
gdrive-bisync --install-service
systemctl --user enable --now gdrive-bisync
```

Confirm that it is healthy:

```bash
gdrive-bisync --status
```

## Everyday usage

### Terminal dashboard

```bash
gdrive-bisync
```

The dashboard contains observability and sync controls only. Use your operating system’s file explorer to browse files.

| Page | Information |
|---|---|
| Overview | Service health, last/next sync, progress, and action totals |
| Activity | Recent uploads, downloads, folder actions, retries, and failures |
| Logs | Live categorized logs with filtering and error-only mode |
| Trash | Recoverable local deletions and restore controls |
| Safety | Deletion limits, backups, notifications, and lock status |
| System | PID, inventories, database size, watcher, and last error |
| Help | Complete keyboard reference |

| Key | Action |
|:---:|---|
| `1`–`7` or `Tab` | Change dashboard page |
| `j` / `k` | Scroll |
| `/` | Filter activity and logs |
| `e` | Show only errors |
| `f` | Hide or show paths |
| `p` | Pause or resume synchronization |
| `s` | Request an immediate sync |
| `d` | Request a safe dry-run preview |
| `x` | Restore an item from trash with confirmation |
| `?` | Open help |
| `q` | Close the dashboard; background sync continues |

### Service commands

| Task | Command |
|---|---|
| Start at login | `systemctl --user enable --now gdrive-bisync` |
| Restart | `systemctl --user restart gdrive-bisync` |
| Stop | `systemctl --user stop gdrive-bisync` |
| Service status | `systemctl --user status gdrive-bisync` |
| Follow service logs | `journalctl --user -u gdrive-bisync -f` |

### Pause and resume

```bash
gdrive-bisync --pause
gdrive-bisync --resume
```

Changes remain on disk while paused and are processed after synchronization resumes.

### Recover a deleted local item

```bash
gdrive-bisync --trash-list
gdrive-bisync --trash-restore ENTRY_ID
```

Restore never overwrites an existing live path. Move the existing item first if you intentionally want to restore the older copy.

## Command-line options

### Synchronization

| Option | Short | Purpose |
|---|:---:|---|
| `--setup` | `-s` | Authenticate with Google Drive |
| `--dry-run` | — | Preview actions without changing files or sync state |
| `--force` | `-f` | Back up and rebuild the local sync database |
| `--allow-unsafe-deletes` | — | Explicitly bypass configured deletion limits for one run |
| `--show-logs` | `-l` | Print informational logs in the terminal |

Always inspect a dry run before bypassing deletion protection:

```bash
gdrive-bisync --dry-run
gdrive-bisync --allow-unsafe-deletes
```

### Status and controls

| Option | Purpose |
|---|---|
| `--status` | Print service state, PID, pause state, task count, last sync, and last error |
| `--pause` | Pause periodic sync and watcher uploads |
| `--resume` | Resume synchronization |
| `--tui` | Explicitly open the terminal dashboard |

### Recovery and service

| Option | Purpose |
|---|---|
| `--trash-list` | List recoverable local deletions |
| `--trash-restore ID` | Restore one trash entry |
| `--install-service` | Install the Linux systemd user service |
| `--uninstall-service` | Stop and remove the Linux systemd user service |

## Configuration

Configuration locations:

| Platform | Directory |
|---|---|
| Linux | `~/.config/gdrive-bisync/config/` |
| Windows | `%USERPROFILE%\.config\gdrive-bisync\config\` |

### Essential options

| Option | Type | Default | Description |
|---|---|---:|---|
| `LOCAL_SYNC_PATH` | string | `~/GoogleDrive` | Local synchronized folder |
| `REMOTE_FOLDER_ID` | string | `root` | Drive folder ID; `root` means My Drive |
| `PERIODIC_SYNC_INTERVAL_MS` | integer | `60000` | Time between Drive checks |
| `WATCH_DEBOUNCE_DELAY` | integer | `5000` | Delay before processing rapid local changes |

### Transfer and retry options

| Option | Type | Default | Description |
|---|---|---:|---|
| `MAX_CONCURRENT_SCANS` | integer | `20` | Concurrent Drive folder scans |
| `MAX_CONCURRENT_DOWNLOADS` | integer | `20` | Simultaneous downloads |
| `MAX_CONCURRENT_UPLOADS` | integer | `10` | Simultaneous uploads |
| `MAX_RETRIES` | integer | `10` | Retry attempts for supported operations |

### Safety and recovery options

| Option | Type | Default | Description |
|---|---|---:|---|
| `MAX_DELETIONS_PER_SYNC` | integer | `20` | Stop when a plan exceeds this deletion count; `0` disables it |
| `MAX_DELETION_PERCENT` | number | `5` | Stop when deletions exceed this percentage; `0` disables it |
| `DATABASE_BACKUP_COUNT` | integer | `5` | Number of local sync-database backups to keep |
| `DESKTOP_NOTIFICATIONS` | boolean | `true` | Show critical failure and recovery notifications on Linux |
| `NOTIFICATION_COOLDOWN_MS` | integer | `1800000` | Suppress repeated notifications for the same failure |
| `ignore` | string[] | `node_modules` | Additional regular-expression patterns to ignore |

### Logging and internal filenames

| Option | Type | Default | Description |
|---|---|---:|---|
| `SHOW_LOGS` | boolean | `false` | Print informational logs and write the daily text log |
| `DB_FILE_NAME` | string | `.gdrive-bisync.db` | Local sync database filename |
| `METADATA_FILE_NAME` | string | `.gdrive-bisync-metadata.json` | Legacy metadata filename kept for compatibility |
| `STATE_FILE_NAME` | string | `.gdrive-bisync-state.json` | Legacy state filename kept for compatibility |

Full example:

```json
{
  "LOCAL_SYNC_PATH": "~/GoogleDrive",
  "REMOTE_FOLDER_ID": "root",
  "DB_FILE_NAME": ".gdrive-bisync.db",
  "WATCH_DEBOUNCE_DELAY": 5000,
  "PERIODIC_SYNC_INTERVAL_MS": 60000,
  "MAX_CONCURRENT_SCANS": 20,
  "MAX_CONCURRENT_DOWNLOADS": 20,
  "MAX_CONCURRENT_UPLOADS": 10,
  "MAX_RETRIES": 10,
  "MAX_DELETIONS_PER_SYNC": 20,
  "MAX_DELETION_PERCENT": 5,
  "DATABASE_BACKUP_COUNT": 5,
  "SHOW_LOGS": false,
  "DESKTOP_NOTIFICATIONS": true,
  "NOTIFICATION_COOLDOWN_MS": 1800000,
  "ignore": ["(^|.*[\\\\/])node_modules([\\\\/].*|$)"]
}
```

Internal state, backups, trash, and partial downloads are ignored automatically.

## Safety and recovery

| Protection | Behavior |
|---|---|
| Dry run | Shows planned work without modifying files or sync state |
| Deletion limits | Stops unexpectedly large deletion plans before execution |
| Recoverable trash | Moves local deletions into a path-preserving trash area |
| Drive trash | Uses Google Drive trash rather than permanent remote deletion |
| Atomic downloads | Keeps the existing file intact if a download fails |
| Database backups | Preserves rotating copies of the previous sync state |
| Single-instance lock | Prevents multiple sync engines from changing the same state |
| Desktop notifications | Alerts you to authentication, database, watcher, and sync failures |

## Troubleshooting

### Check current health

```bash
gdrive-bisync --status
systemctl --user status gdrive-bisync
```

### View recent errors

Open the dashboard and select **Logs**, or run:

```bash
journalctl --user -u gdrive-bisync -n 100
```

### The service is already running

This is expected when systemd owns the background sync process. Run `gdrive-bisync` in a terminal to open the dashboard, or use `--status`; do not start a second daemon manually.

### Local and Drive contents look different

Request a safe comparison from the dashboard with `d`, or stop the service temporarily and run:

```bash
systemctl --user stop gdrive-bisync
gdrive-bisync --dry-run
systemctl --user start gdrive-bisync
```

### Desktop notifications do not appear

Install `notify-send` and ensure a graphical notification session is available. Synchronization continues normally when desktop notifications are unavailable; errors remain visible in status, logs, and the TUI.
