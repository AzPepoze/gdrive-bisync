# gdrive-bisync

A bidirectional synchronization utility between a local directory and a Google Drive folder.

## Table of Contents
- [Quick Start](#quick-start-get-started-in-3-steps)
- [Detailed Installation](#detailed-installation)
- [Usage & CLI Options](#usage--cli-options)
- [Configuration Guide](#configuration-guide)
- [How it Works](#how-it-works)

---

## Quick Start (Get Started in 3 Steps)

### Step 1: Set up your Google API Credentials
Before running the utility, you need to obtain authorization keys from Google:
1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Enable the **Google Drive API** for your project.
3. Go to the **Credentials** tab, click **Create Credentials** -> **OAuth client ID**, choose **Desktop app**, and download the generated JSON key.
4. Rename the downloaded file to `credentials.json` and move it to:
   - **Linux:** `~/.config/gdrive-bisync/config/credentials.json`
   - **Windows:** `%USERPROFILE%\.config\gdrive-bisync\config\credentials.json`

### Step 2: Configure your Sync Settings
Create a file named `config.json` in the same directory as your `credentials.json` with your folder paths:
```json
{
  "LOCAL_SYNC_PATH": "~/GoogleDrive",
  "REMOTE_FOLDER_ID": "root"
}
```
> [!TIP]
> To sync a specific folder, open the folder in Google Drive, copy the ID from the URL (e.g., `https://drive.google.com/drive/folders/YOUR_FOLDER_ID`), and paste it in `"REMOTE_FOLDER_ID"`.

### Step 3: Run the Authentication & Sync
Authorize the application on your Google account:
```bash
./gdrive-bisync --setup
```
This opens your browser to complete Google OAuth. Once finished, start the synchronizer:
```bash
./gdrive-bisync
```

---

## Detailed Installation

### Arch Linux (AUR)
```bash
paru -S gdrive-bisync
# or
yay -S gdrive-bisync
```

### Manual Build (From Source)
Ensure you have Go 1.25+ installed:
```bash
git clone https://github.com/AzPepoze/gdrive-bisync
cd gdrive-bisync
make
```
This creates the `gdrive-bisync` binary in the current directory.

---

## Usage & CLI Options

```bash
# Authorize the app for first-time use
./gdrive-bisync --setup

# Run the sync engine (foreground daemon)
./gdrive-bisync

# Preview actions without changing local or remote files
./gdrive-bisync --dry-run

# Inspect and control the running service
./gdrive-bisync --status
./gdrive-bisync --pause
./gdrive-bisync --resume

# List or restore recoverable local deletions
./gdrive-bisync --trash-list
./gdrive-bisync --trash-restore ENTRY_ID

# Open the Bubble Tea terminal manager
./gdrive-bisync --tui

# Reset local sync state and force a complete rescan
./gdrive-bisync --force

# Install systemd service for automatically running in background (Linux only)
./gdrive-bisync --install-service

# Uninstall systemd service (Linux only)
./gdrive-bisync --uninstall-service
```

### Linux Service Management (systemd)
Once installed with `./gdrive-bisync --install-service`, you can control it via systemd:
```bash
# Enable automatic startup and run the service
systemctl --user enable --now gdrive-bisync

# Check if the service is running and view recent logs
systemctl --user status gdrive-bisync

# View live log outputs
journalctl --user -u gdrive-bisync -f
```

---

## Configuration Guide

Place `config.json` inside your configurations folder (`~/.config/gdrive-bisync/config/`).

### Configuration Options

| Option | Type | Default | Description |
|---|---|---|---|
| `LOCAL_SYNC_PATH` | string | `~/GoogleDrive` | The local folder to sync. |
| `REMOTE_FOLDER_ID` | string | `root` | Google Drive folder ID to sync with (`root` targets the main drive). |
| `WATCH_DEBOUNCE_DELAY` | integer | `5000` | Delay in milliseconds before uploading local changes (prevents rapid-save spam). |
| `PERIODIC_SYNC_INTERVAL_MS` | integer | `60000` | How often the engine polls Google Drive for changes (default: 1 minute). |
| `MAX_CONCURRENT_SCANS` | integer | `20` | Maximum parallel scans during remote checks. |
| `MAX_CONCURRENT_DOWNLOADS` | integer | `20` | Maximum concurrent downloads. |
| `MAX_CONCURRENT_UPLOADS` | integer | `10` | Maximum concurrent uploads. |
| `MAX_DELETIONS_PER_SYNC` | integer | `20` | Abort before destructive reconciliation if more deletions are planned. `0` disables this limit. |
| `MAX_DELETION_PERCENT` | number | `5` | Abort if deletions exceed this percentage of the local scan. `0` disables this limit. |
| `DATABASE_BACKUP_COUNT` | integer | `5` | Number of rotating database backups to retain. |
| `ignore` | string[] | `["(^\|.*[\\\\/])node_modules([\\\\/].*|$)"]` | Regex patterns of paths to ignore. |

---

## How it Works

### 1. Bidirectional Sync logic
- **Local changes**: Monitored in real-time. When you modify or create a file locally, the filesystem watcher triggers an upload after the debounce delay.
- **Remote changes**: Google Drive is polled periodically. Only files modified since the last check are updated to conserve network bandwidth.
- **Self-trigger prevention**: The engine ignores local filesystem write/create events that are caused by its own downloads to prevent infinite upload/download loops.
- **Single instance**: An OS file lock prevents a service and manual process from syncing the same state concurrently.
- **Safe deletion**: Destructive changes are preflighted against count and percentage thresholds before reconciliation.
- **Atomic downloads**: Downloads use a temporary sibling file and replace the destination only after a successful flush.

### 2. Databases & Files Created
Inside your `LOCAL_SYNC_PATH`, the utility maintains:
- `.gdrive-bisync.db`: bbolt database tracking remote files and cached metadata.
- `.gdrive-bisync-metadata.json`: Holds mapping state to verify whether files changed locally or remotely.
- `.gdrive-bisync-backups/`: Rotating state backups created before startup.
- `.trash/<date>/<entry-id>/`: Path-preserving recoverable payloads and manifests.

Runtime status, pause state, and the instance lock live under the user configuration directory in `gdrive-bisync/runtime/`. `--allow-unsafe-deletes` bypasses deletion thresholds for one explicitly requested run.

Console logs infer a category from the calling package. Category colors are generated dynamically from a stable hash, so new packages receive consistent colors without maintaining a category table. Progress animation is disabled automatically when output is not an interactive terminal.
