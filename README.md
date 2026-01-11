# gdrive-bisync (Go Version)

`gdrive-bisync` is a high-performance command-line utility for synchronizing a local directory with a Google Drive folder. Rewritten in Go, it offers robust features, low resource usage, and single-binary deployment.

## Features

- **Efficient Syncing:** Uses the **Google Drive Changes API** for fast, incremental updates after the initial scan.
- **Real-time Monitoring:** Instantly detects local file changes (edits, additions, deletions) and uploads them.
- **Parallel Scanning:** Highly optimized remote scanning with configurable concurrency and retry logic.
- **Smart Conflict Handling:** Detects remote folder deletions and reflects them locally to prevent loops.
- **State Persistence:** Remembers where it left off, allowing for instant startups even after restarts.
- **Cross-Platform:** Runs seamlessly on Linux and Windows.

## Installation

### From Releases

1.  Download the latest release for your OS from the [Releases Page](../../releases).
    *   `linux-gdrive-bisync.zip`
    *   `windows-gdrive-bisync.zip`
2.  Extract the archive.
    *   **Linux:** Contains binary, config folder, and `setup_service.sh`.
    *   **Windows:** Contains executable and config folder.

### Building from Source

**Prerequisites:** Go 1.21+, Make (optional, for automation), Zip (optional, for packaging).

1.  Clone the repository:
    ```bash
    git clone https://github.com/AzPepoze/gdrive-bisync
    cd gdrive-bisync
    ```

2.  **Using Make (Recommended):**
    This will compile binaries for both Linux and Windows and create release archives.
    ```bash
    make
    ```
    Output will be in `dist/` (unpacked) and `release/` (zipped).

3.  **Manual Build:**
    ```bash
    go build -o gdrive-bisync cmd/gdrive-bisync/main.go
    ```

## Development

To run the application directly from source during development:

```bash
make dev
```
You can also pass arguments (like `--force` or `--setup`) using environment variables or by calling `go run` directly:
```bash
go run cmd/gdrive-bisync/main.go --force
```

## Setup & Configuration

### 1. Google Credentials
You need your own Google Cloud credentials to allow the app to access your Drive.

1.  Go to the [Google Cloud Console](https://console.cloud.google.com/).
2.  Create a project and enable the **Google Drive API**.
3.  Go to **Credentials** -> **Create Credentials** -> **OAuth client ID**.
4.  Select **Desktop app**.
5.  Download the JSON file.
6.  **Rename** it to `credentials.json` and place it inside the `config/` folder next to the binary.

### 2. Application Config
1.  Copy the example config:
    ```bash
    cp config/config.example.json config/config.json
    ```
2.  Edit `config/config.json`:
    ```json
    {
      "LOCAL_SYNC_PATH": "~/GoogleDrive",
      "REMOTE_FOLDER_ID": "root",
      "WATCH_DEBOUNCE_DELAY": 5000,
      "PERIODIC_SYNC_INTERVAL_MS": 60000,
      "MAX_CONCURRENT_SCANS": 20,
      "MAX_RETRIES": 10,
      "ignore": [
        "(^|.*[\\/])node_modules([\\/].*|$)"
      ]
    }
    ```
    *   `LOCAL_SYNC_PATH`: Where your files are locally.
    *   `REMOTE_FOLDER_ID`: The Drive folder ID (or "root").
    *   `MAX_CONCURRENT_SCANS`: API concurrency limit (default: 20).

### 3. First Run (Authentication)
Run the application with the setup flag to authorize your account:

**Linux:**
```bash
./gdrive-bisync --setup
```

**Windows:**
```powershell
.\gdrive-bisync.exe --setup
```

The app will open your browser. Login and the app will capture the token automatically.

## Usage

Once configured, simply run the binary to start the sync service:

```bash
./gdrive-bisync
```

**Options:**
*   `--setup`: Run first-time authentication.
*   `--force`: Delete local metadata and state files to force a fresh re-scan.

To stop the application, press `Ctrl+C`. It will save its state safely before exiting.

### Linux Service (systemd)

A helper script is included in the Linux release to install the application as a background service.

```bash
sudo ./setup_service.sh
```

