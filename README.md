# az-gdrive: Google Drive Bisync

`az-gdrive` is a command-line utility designed to synchronize a local directory with a Google Drive folder. It provides robust features for keeping your local files and Google Drive in sync, including periodic scans, real-time local change detection, and a user-friendly console interface.

I'm doing this project for Bisync google drive for linux.

## Features

-    **Bidirectional Sync:** Keeps local and remote folders synchronized.
-    **Real-time Local Change Detection:** Automatically detects and syncs changes (additions, modifications, deletions) in your local directory.
-    **Periodic Full Sync:** Performs a full scan and synchronization at configurable intervals to ensure consistency.
-    **Configurable Ignore Patterns:** Exclude specific files or folders from synchronization using regular expressions.

## Installation

To set up `az-gdrive`, follow these steps:

1. **Clone the repository:**
     ```bash
     git clone https://github.com/AzPepoze/az-gdrive
     cd az-gdrive
     ```
2. **Install dependencies:**
     ```bash
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
     az-gdrive/
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
     pnpm authenticate
     ```

     Your browser should open automatically. Follow the prompts to log in and grant permissions. The application will automatically capture the authorization code.

### Config Folder Structure

Then, after running `pnpm authenticate` and creating `config.json` (if applicable), it will contain:

```
az-gdrive/
├── config/
│   ├── credentials.json
│   ├── config.json
│   └── token.json
└── ... (other project files)
```

## Configuration

You can configure `az-gdrive` by creating a `config.json` file in the `config` directory.

Here's an example `config.json` with default values:

```json
{
	"LOCAL_SYNC_PATH": "~/GoogleDrive",
	"REMOTE_FOLDER_ID": "root",
	"METADATA_FILE_NAME": ".az-gdrive-sync-metadata.json",
	"WATCH_DEBOUNCE_DELAY": 5000,
	"PERIODIC_SYNC_INTERVAL_MS": 60000,
	"ignore": ["(^|.*[\\/])node_modules([\\/].*|$)"]
}
```

-    `LOCAL_SYNC_PATH`: The local directory to synchronize (e.g., `~/GoogleDrive2`).
-    `REMOTE_FOLDER_ID`: The Google Drive folder ID to synchronize with. Use `"root"` for your main Drive folder.
-    `METADATA_FILE_NAME`: Name of the metadata file used for sync tracking.
-    `WATCH_DEBOUNCE_DELAY`: Delay (in ms) before processing local file changes.
-    `PERIODIC_SYNC_INTERVAL_MS`: Interval (in ms) for full periodic syncs.
-    `ignore`: An array of regular expression strings for files/folders to ignore during sync.

## Usage

To update the application to the latest version, build it, and then start it, you can use the provided convenience script:

```bash
./update_and_start.sh
```

This script will perform the following actions:

1. Pull the latest changes from your Git repository (`git pull`).
2. Rebuild the project (`pnpm build`).
3. Start the application (`pnpm start`).
