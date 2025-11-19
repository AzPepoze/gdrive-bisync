# gdrive-bisync: Google Drive Bisync

`gdrive-bisync` is a command-line utility designed to synchronize a local directory with a Google Drive folder. It provides robust features for keeping your local files and Google Drive in sync, including periodic scans, real-time local change detection, and a user-friendly console interface.

I'm doing this project for Bisync google drive for linux.

## Features

- **Bidirectional Sync:** Keeps local and remote folders synchronized.
- **Real-time Local Change Detection:** Automatically detects and syncs changes (additions, modifications, deletions) in your local directory.
- **Periodic Full Sync:** Performs a full scan and synchronization at configurable intervals to ensure consistency. By default, it will sleep for 60 seconds (or the value set in `PERIODIC_SYNC_INTERVAL_MS` in `config.json`) before the next scan.
- **Configurable Ignore Patterns:** Exclude specific files or folders from synchronization using regular expressions.

## Prerequisites

Before you begin, ensure you have the following installed on your system:

- **Node.js:** `gdrive-bisync` is a Node.js application. You will need Node.js (at least v18.12) to run it. You can download it from the [official Node.js website](https://nodejs.org/).
- **Git:** You will need Git to clone the repository. You can download it from the [Git website](https://git-scm.com/downloads).

## Installation

To set up `gdrive-bisync`, follow these steps:

1. **Clone the repository:**
   ```bash
   git clone https://github.com/AzPepoze/gdrive-bisync
   cd gdrive-bisync
   ```
2. **Install dependencies:**
   ```bash
   # Using npm
   npm install

   # Or using pnpm
   pnpm install
   ```

## Authentication with Google Drive

Before running the sync, you need to authenticate with your Google account.

1. **Obtain `credentials.json`:**

   - Go to the Google Cloud Console: [https://console.cloud.google.com/](https://console.cloud.google.com/)
   - Create a new project or select an existing one.
   - In the API Library, search for and enable the "**Google Drive API**".
   - Go to "**Credentials**" -> "**Create Credentials**" -> "**OAuth client ID**".
   - Select "**Desktop app**" as the application type.
   - Download the JSON file provided after creation.
   - **IMPORTANT:** First, create a `config` directory in the project root. Then, rename the downloaded file to `credentials.json` and place it inside the newly created `config` directory.

   After placing your `credentials.json`, your `config` folder should look like this:

   ```
   gdrive-bisync/
   ├── config/
   │   └── credentials.json
   └── ... (other project files)
   ```
2. **Configure Redirect URI:**

   - In the Google Cloud Console, under your "OAuth 2.0 Client ID for Desktop app", find "Authorized redirect URIs".
   - Click "**ADD URI**" and enter `http://localhost:3000` (or another port of your choice, but ensure it's a high-numbered port not commonly used).
   - Save the changes.
   - **Crucially:** Ensure the `redirect_uris` array in your local `credentials.json` file also contains the exact same URI (e.g., `["http://localhost:3000"]`).
3. **Run the authentication command:**

   ```bash
   # Using npm
   npm run authenticate

   # Or using pnpm
   pnpm authenticate
   ```

   Your browser should open automatically. Follow the prompts to log in and grant permissions. The application will automatically capture the authorization code.

### Config Folder Structure

Then, after running `pnpm authenticate` and creating `config.json` (if applicable), it will contain:

```
gdrive-bisync/
├── config/
│   ├── credentials.json
│   ├── config.json
│   └── token.json
└── ... (other project files)
```

## Configuration

You can configure `gdrive-bisync` by creating a `config.json` file in the `config` directory.

Here's an example `config.json` with default values:

```json
{
	"LOCAL_SYNC_PATH": "~/GoogleDrive",
	"REMOTE_FOLDER_ID": "root",
	"METADATA_FILE_NAME": ".gdrive-bisync-sync-metadata.json",
	"WATCH_DEBOUNCE_DELAY": 5000,
	"PERIODIC_SYNC_INTERVAL_MS": 60000,
	"ignore": ["(^|.*[\\/])node_modules([\\/].*|$)"]
}
```

- `LOCAL_SYNC_PATH`: The local directory to synchronize (e.g., `~/GoogleDrive2`).
- `REMOTE_FOLDER_ID`: The Google Drive folder ID to synchronize with. Use `"root"` for your main Drive folder.
- `METADATA_FILE_NAME`: Name of the metadata file used for sync tracking.
- `WATCH_DEBOUNCE_DELAY`: Delay (in ms) before processing local file changes.
- `PERIODIC_SYNC_INTERVAL_MS`: Interval (in ms) for full periodic syncs.
- `ignore`: An array of regular expression strings for files/folders to ignore during sync.



## Running as a Service (Linux with systemd)

For continuous background synchronization, you can set up `gdrive-bisync` as a `systemd` service. This will ensure the application automatically starts on boot and restarts if it fails.

A convenience script, `setup_service.sh`, is provided to automate this process.

### How to Use

1. **Run the setup script:**
   ```bash
   sudo ./setup_service.sh
   ```

   The script will:- Create a `gdrive-bisync.service` file.
   - Move it to the systemd directory (`/etc/systemd/system/`).
   - Reload the systemd daemon.
   - Enable and start the service.

### Managing the Service

- **Check the status:**

  ```bash
  sudo systemctl status gdrive-bisync
  ```
- **View logs:**

  ```bash
  journalctl -u gdrive-bisync -f
  ```
- **Stop the service:**

  ```bash
  sudo systemctl stop gdrive-bisync
  ```

## Manual Testing/Execution

For manual testing or execution, you can build and run the project directly.

First, build the project to ensure all TypeScript files are compiled to JavaScript:

```bash
# Using npm
npm run build

# Or using pnpm
pnpm build
```

Once the build is complete, you can start the synchronization process:

```bash
# Using npm
npm start

# Or using pnpm
pnpm start
```

### Convenience Script (`update_and_start.sh`)

A convenience script is provided to automate the entire process of updating, building, and starting the application. It will automatically detect if `pnpm` is installed and use it. If not, it will fall back to using `npm`.

```bash
./update_and_start.sh
```

This script will perform the following actions:

1. Pull the latest changes from your Git repository (`git pull`).
2. Install dependencies (using `pnpm install` or `npm install`).
3. Rebuild the project (using `pnpm build` or `npm run build`).
4. Start the application (using `pnpm start` or `npm start`).