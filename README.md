# gdrive-bisync

`gdrive-bisync` is a command-line utility for bidirectional synchronization between a local directory and a Google Drive folder. It offers robust features, low resource usage, and single-binary deployment.

## Table of Contents

-    [Features](#features)
-    [Installation](#installation)
     -    [Arch Linux (AUR)](#arch-linux-aur)
     -    [From Releases](#from-releases)
     -    [Building from Source](#building-from-source)
-    [Usage](#usage)
-    [Setup & Configuration](#setup--configuration)
     -    [Configuration File Locations](#configuration-file-locations)
     -    [1. Google Credentials](#1-google-credentials)
     -    [2. Application Config](#2-application-config)
     -    [3. First Run (Authentication)](#3-first-run-authentication)
     -    [How It Works](#how-it-works)
     -    [Log Files](#log-files)
-    [Development](#development)

## Features

-    **Efficient Syncing:** Uses the **Google Drive Changes API** for fast, incremental updates after the initial scan.
-    **Real-time Monitoring:** Instantly detects local file changes (edits, additions, deletions) and uploads them.
-    **Parallel Scanning:** Highly optimized remote scanning with configurable concurrency and retry logic.
-    **Smart Conflict Handling:** Detects remote folder deletions and reflects them locally to prevent loops.
-    **State Persistence:** Remembers where it left off, allowing for instant startups even after restarts.
-    **Cross-Platform:** Runs seamlessly on Linux and Windows.

# Installation

## Arch Linux (AUR)

You can install `gdrive-bisync` from the AUR using your favorite AUR helper:

```bash
yay -S gdrive-bisync
# or
paru -S gdrive-bisync
```

> [!WARNING]
> When uninstalling the package, your configuration files in `~/.config/gdrive-bisync/` are preserved. Remove them manually if needed.

## From Releases

1. Download the latest release for your OS from the [Releases Page](../../releases).
     - `linux-gdrive-bisync.zip`
     - `windows-gdrive-bisync.zip`
2. Extract the archive.
     - **Linux:** Contains binary, config folder, and `setup_service.sh`.
     - **Windows:** Contains executable and config folder.

## Usage

### Basic Usage

```bash
./gdrive-bisync
```

### Command-Line Options

| Flag                  | Description                                                                                                                                                                                                     |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--setup`             | Run first-time Google OAuth authentication. Opens a browser window to authorize the application. Must be run before first sync.                                                                                 |
| `--force`             | Force a complete rescan by deleting state and metadata files. Use this if sync state becomes corrupted or you want to start fresh. **Warning**: This will trigger a full rescan of both local and remote files. |
| `--install-service`   | Install systemd user service for automatic startup (Linux only). The service will use the current executable path.                                                                                              |
| `--uninstall-service` | Uninstall systemd user service (Linux only). Stops and removes the service.                                                                                                                                     |

### Examples

```bash
# First time setup
./gdrive-bisync --setup

# Normal operation
./gdrive-bisync

# Reset and rescan everything
./gdrive-bisync --force

# Install as systemd service (Linux only)
./gdrive-bisync --install-service

# Uninstall systemd service (Linux only)
./gdrive-bisync --uninstall-service
```

### Linux Service Management

After installing the service with `--install-service`, you can manage it using systemctl:

```bash
# Enable and start the service
systemctl --user enable --now gdrive-bisync

# Check service status
systemctl --user status gdrive-bisync

# View real-time logs
journalctl --user -u gdrive-bisync -f

# Stop the service
systemctl --user stop gdrive-bisync

# Disable the service
systemctl --user disable gdrive-bisync

# Restart the service
systemctl --user restart gdrive-bisync
```

# Setup & Configuration

## Configuration File Locations

The application searches for configuration files in the following order:

### Linux

1. **`~/.config/gdrive-bisync/config/`** (Preferred - User configuration directory)
     - This is checked first if it exists and contains `config.json`
     - Full path: `/home/<username>/.config/gdrive-bisync/config/`
2. **`./config/`** (Fallback - Local directory)
     - Used if the user config directory doesn't exist or doesn't contain `config.json`
     - Located relative to where you run the application

### Windows

1. **`%USERPROFILE%\.config\gdrive-bisync\config\`** (Preferred - User configuration directory)
     - This is checked first if it exists and contains `config.json`
     - Example: `C:\Users\YourUsername\.config\gdrive-bisync\config\`
     - Note: Uses `.config` folder in Windows home directory (not the standard AppData location)
2. **`.\config\`** (Fallback - Local directory)
     - Used if the user config directory doesn't exist or doesn't contain `config.json`
     - Located relative to where you run the application

### Required Files in Config Directory

Both `credentials.json` (Google OAuth) and `config.json` (application settings) must be placed in the same configuration directory.

## 1. Google Credentials

1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Enable **Google Drive API**.
3. Create **OAuth client ID** (Desktop app) and download the JSON.
4. Rename it to `credentials.json` and place it in your configuration directory.

## 2. Application Config

Create or edit `config.json` in your configuration directory with the following settings:

```json
{
	"LOCAL_SYNC_PATH": "~/GoogleDrive",
	"REMOTE_FOLDER_ID": "root",
	"METADATA_FILE_NAME": ".gdrive-bisync-metadata.json",
	"STATE_FILE_NAME": ".gdrive-bisync-state.json",
	"WATCH_DEBOUNCE_DELAY": 5000,
	"PERIODIC_SYNC_INTERVAL_MS": 60000,
	"MAX_CONCURRENT_SCANS": 20,
	"MAX_RETRIES": 10,
	"ignore": ["(^|.*[\\\\/])node_modules([\\\\/].*|$)"]
}
```

### Configuration Options Explained

| Option                      | Type     | Default                        | Description                                                                                                                |
| --------------------------- | -------- | ------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| `LOCAL_SYNC_PATH`           | string   | `~/GoogleDrive`                | Local directory to sync. Use `~` for home directory (works on both Linux and Windows).                                     |
| `REMOTE_FOLDER_ID`          | string   | `root`                         | Google Drive folder ID to sync with. Use `root` for the root of your Drive, or find a specific folder ID in the Drive URL. |
| `METADATA_FILE_NAME`        | string   | `.gdrive-bisync-metadata.json` | Hidden file storing local file metadata. Created in `LOCAL_SYNC_PATH`. Automatically ignored during sync.                  |
| `STATE_FILE_NAME`           | string   | `.gdrive-bisync-state.json`    | Hidden file storing sync state and Drive API page token. Created in `LOCAL_SYNC_PATH`. Automatically ignored during sync.  |
| `WATCH_DEBOUNCE_DELAY`      | integer  | `5000`                         | Milliseconds to wait after detecting a local file change before syncing (prevents multiple uploads during rapid edits).    |
| `PERIODIC_SYNC_INTERVAL_MS` | integer  | `60000`                        | Milliseconds between automatic remote sync checks (default: 1 minute).                                                     |
| `MAX_CONCURRENT_SCANS`      | integer  | `20`                           | Maximum parallel threads when scanning remote Drive folders. Higher values = faster initial scan but more API calls.       |
| `MAX_RETRIES`               | integer  | `10`                           | Number of retry attempts for failed API operations.                                                                        |
| `ignore`                    | string[] | See below                      | Array of regex patterns for files/folders to ignore. The metadata and state files are automatically added to this list.    |

**Default ignore pattern:** `["(^|.*[\\/])node_modules([\\/].*|$)"]` - This ignores all `node_modules` folders.

**Example: Sync a specific folder**

1. Open the folder in Google Drive
2. Copy the folder ID from the URL: `https://drive.google.com/drive/folders/ABC123XYZ`
3. Set `"REMOTE_FOLDER_ID": "ABC123XYZ"` in config.json

See the full [example configuration file](./config/config.example.json) for more details.

## 3. First Run (Authentication)

Run with the setup flag to authorize the application with Google:

```bash
./gdrive-bisync --setup
```

This will:

1. Open your default web browser
2. Ask you to log in to your Google account
3. Request permission to access Google Drive
4. Save the authentication token in your config directory

The token is stored as `token.json` in the same directory as your `credentials.json` and `config.json`.

## How It Works

### Initial Sync

On first run (or after `--force`):

1. Scans your local `LOCAL_SYNC_PATH` directory
2. Scans the remote Google Drive folder (`REMOTE_FOLDER_ID`)
3. Compares files and synchronizes in both directions
4. Saves state to `.gdrive-bisync-state.json` for future incremental syncs

### Incremental Sync

After the initial sync:

1. **Local Changes**: Monitored in real-time using file system watchers
     - Debounced by `WATCH_DEBOUNCE_DELAY` to avoid multiple uploads during rapid edits
2. **Remote Changes**: Uses Google Drive Changes API for efficient incremental updates
     - Checked every `PERIODIC_SYNC_INTERVAL_MS` (default: 1 minute)
     - Only fetches changes since the last `pageToken`, not the entire drive

### State Persistence

Two hidden files are created in your `LOCAL_SYNC_PATH`:

-    **`.gdrive-bisync-metadata.json`**: Stores local file metadata (mod times, hashes, etc.)
-    **`.gdrive-bisync-state.json`**: Stores remote file state and the Drive API page token

These files allow the app to:

-    Resume syncing instantly after restart without rescanning
-    Detect conflicts and deletions accurately
-    Track which files have been synced

### Conflict Handling

-    **Local folder deleted, remote exists**: Remote changes are downloaded (folder recreated)
-    **Remote folder deleted, local exists**: Local folder is deleted to match remote
-    **File modified in both locations**: Last modified timestamp wins (newer file overwrites older)

## Log Files

The application creates detailed log files for debugging and monitoring:

### Linux

1. **`/tmp/gdrive-bisync-logs/`** (Preferred)
     - Automatically cleaned by the system on reboot
     - Used if the directory can be created successfully
2. **`./logs/`** (Fallback)
     - Used if `/tmp/gdrive-bisync-logs/` cannot be created
     - Located relative to where you run the application

### Windows

-    **`.\logs\`** (Always used)
     -    Located relative to where you run the application

### Log File Format

-    **Filename**: `sync-YYYY-MM-DD.log` (e.g., `sync-2026-01-12.log`)
-    **Console Output**: INFO level and above (colored, user-friendly)
-    **File Output**: DEBUG level and above (detailed, all operations)
-    Logs are appended daily to the same file

### Viewing Logs

```bash
# Linux - View latest log
tail -f /tmp/gdrive-bisync-logs/sync-$(date +%Y-%m-%d).log

# Linux - View logs if running as systemd service
journalctl --user -u gdrive-bisync -f

# Windows - View latest log
Get-Content .\logs\sync-$(Get-Date -Format "yyyy-MM-dd").log -Wait
```

### Building from Source

**Prerequisites:** Go 1.21+, Make (optional).

1. Clone the repository:
     ```bash
     git clone https://github.com/AzPepoze/gdrive-bisync
     cd gdrive-bisync
     ```
2. **Using Make (Recommended):**
     ```bash
     make
     ```
3. **Manual Build:**
     ```bash
     go build -o gdrive-bisync cmd/gdrive-bisync/main.go
     ```

## Development

```bash
go run cmd/gdrive-bisync/main.go --force
```
