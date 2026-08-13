# gdrive-bisync

Keep a folder on your computer in sync with Google Drive.

It runs quietly in the background and includes a terminal dashboard for checking activity, errors, and deleted files.

## Install

Arch Linux:

```bash
paru -S gdrive-bisync
```

From source:

```bash
git clone https://github.com/AzPepoze/gdrive-bisync.git
cd gdrive-bisync
go build -o gdrive-bisync ./cmd/gdrive-bisync
sudo install -m 0755 gdrive-bisync /usr/local/bin/gdrive-bisync
```

## Set up

1. Enable the Google Drive API in the [Google Cloud Console](https://console.cloud.google.com/).
2. Create an OAuth client for a **Desktop app**.
3. Download its JSON file.
4. Run:

```bash
mkdir -p ~/.config/gdrive-bisync/config
cp ~/Downloads/YOUR_FILE.json ~/.config/gdrive-bisync/config/credentials.json
```

5. Create `~/.config/gdrive-bisync/config/config.json`:

On Windows, use `%USERPROFILE%\.config\gdrive-bisync\config\` instead.

```json
{
  "LOCAL_SYNC_PATH": "~/GoogleDrive",
  "REMOTE_FOLDER_ID": "root"
}
```

6. Connect your Google account:

```bash
gdrive-bisync --setup
```

7. Preview the first sync:

```bash
gdrive-bisync --dry-run
```

## Run

Install the background service:

```bash
gdrive-bisync --install-service
systemctl --user enable --now gdrive-bisync
```

Open the dashboard:

```bash
gdrive-bisync
```

The dashboard shows sync health, progress, recent activity, logs, trash, and safety information.

| Key | Action |
|:---:|---|
| `1`–`7` | Change page |
| `s` | Sync now |
| `d` | Preview a dry run |
| `p` | Pause or resume |
| `f` | Hide or show file paths |
| `x` | Restore from trash |
| `?` | Help |
| `q` | Close the dashboard |

## Useful commands

| Command | What it does |
|---|---|
| `gdrive-bisync --status` | Show current sync status |
| `gdrive-bisync --dry-run` | Preview changes safely |
| `gdrive-bisync --pause` | Pause syncing |
| `gdrive-bisync --resume` | Resume syncing |
| `gdrive-bisync --trash-list` | Show recoverable deleted files |
| `gdrive-bisync --trash-restore ID` | Restore a deleted file |
| `journalctl --user -u gdrive-bisync -f` | Follow service logs |

Run `gdrive-bisync --help` to see every option.

## Configuration

Most users only need these two settings:

| Setting | Default | Meaning |
|---|---:|---|
| `LOCAL_SYNC_PATH` | `~/GoogleDrive` | Folder on your computer |
| `REMOTE_FOLDER_ID` | `root` | Google Drive folder to sync |

`root` means your entire **My Drive**. To sync one folder, use the ID from its URL:

```text
https://drive.google.com/drive/folders/FOLDER_ID
```

Common optional settings:

| Setting | Default | Meaning |
|---|---:|---|
| `PERIODIC_SYNC_INTERVAL_MS` | `60000` | Check Drive every 60 seconds |
| `MAX_DELETIONS_PER_SYNC` | `20` | Stop unexpectedly large deletions |
| `MAX_DELETION_PERCENT` | `5` | Maximum deletion percentage |
| `DATABASE_BACKUP_COUNT` | `5` | Number of state backups to keep |
| `DESKTOP_NOTIFICATIONS` | `true` | Show important Linux notifications |
| `ignore` | `node_modules` | Extra path patterns to ignore |

See [config/config.example.json](config/config.example.json) for every setting.

## Safety

- Use `--dry-run` whenever you are unsure.
- Large deletion plans stop automatically.
- Local deletions go to recoverable trash.
- Remote deletions use Google Drive trash.
- Downloads and sync-state updates are atomic.

For development information, see [CONTRIBUTING.md](CONTRIBUTING.md).
