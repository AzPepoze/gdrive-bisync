import { promises as fs } from "fs";
import * as path from "path";
import logger from "./logger";
import { authorize } from "./authenticate";
import { listFilesRecursive, downloadFile, uploadOrUpdateFile, createFolder, deleteFile, DriveFile } from "./driveApi";
import { ui } from "./ui";
import { resolvePath } from "./utils";
import { getLocalFilesRecursive, LocalFile } from "./localScanner";
import { retryOperation } from "./utils";
import * as chokidar from "chokidar";
import { OAuth2Client } from "google-auth-library";

// --- CONFIGURATION ---
const LOCAL_SYNC_PATH = "~/GoogleDrive2";
const REMOTE_FOLDER_ID = "root";
const METADATA_FILE_NAME = ".az-gdrive-sync-metadata.json";
const WATCH_DEBOUNCE_DELAY = 5000; // 5 seconds debounce for file changes
const CONFIG_PATH = path.join(process.cwd(), "config.json");

interface FileMetadata {
	remoteMd5Checksum?: string;
}

interface Config {
	ignore?: string[];
}

async function loadConfig(): Promise<Config> {
	try {
		const content = await fs.readFile(CONFIG_PATH, "utf8");
		return JSON.parse(content);
	} catch (err) {
		logger.warn("No config.json found or error reading config. Using default settings.");
		return {};
	}
}

// Enum to make sync decisions clearer
enum SyncAction {
	DOWNLOAD_NEW,
	DOWNLOAD_UPDATE,
	UPLOAD_NEW,
	UPLOAD_UPDATE,
	UPLOAD_CONFLICT,
	SKIP_IDENTICAL,
	SKIP_NO_CHANGE,
}

// A task to be executed
interface SyncTask {
	action: SyncAction;
	filePath: string;
}

/**
 * Determines the required sync action for a single file.
 */
function determineSyncAction(
	filePath: string,
	localFile: LocalFile | undefined,
	remoteFile: DriveFile | undefined,
	metadata: Map<string, FileMetadata>
): SyncAction {
	const lastSyncedInfo = metadata.get(filePath);

	if (localFile && !remoteFile) {
		return SyncAction.UPLOAD_NEW;
	}

	if (!localFile && remoteFile) {
		return SyncAction.DOWNLOAD_NEW;
	}

	if (localFile && remoteFile) {
		// If MD5 checksums are available and match, files are identical.
		if (localFile.md5Checksum && remoteFile.md5Checksum && localFile.md5Checksum === remoteFile.md5Checksum) {
			return SyncAction.SKIP_IDENTICAL;
		}

		// If remote file has changed since last sync (or no last sync info).
		// If local checksum is different from remote, it's a conflict or remote is newer.
		const lastSyncedRemoteMd5 = lastSyncedInfo?.remoteMd5Checksum;

		if (lastSyncedRemoteMd5 === remoteFile.md5Checksum) {
			// Remote file has not changed since last sync. Local is newer or different.
			return SyncAction.UPLOAD_UPDATE;
		} else {
			// Remote file has changed since last sync.
			// If local checksum is different from remote, it's a conflict or remote is newer.
			// As a fallback for conflict resolution, we can use modifiedTime if MD5s are not conclusive.
			const localTime = new Date(localFile.mtime).getTime();
			const remoteTime = new Date(remoteFile.modifiedTime).getTime();
			const buffer = 2000; // 2-second buffer for timestamp comparison

			if (remoteTime > localTime + buffer) {
				return SyncAction.DOWNLOAD_UPDATE;
			} else {
				// Local is newer or we can't decide based on time, treat as conflict for now.
				return SyncAction.UPLOAD_CONFLICT; // Local version wins in this implementation
			}
		}
	}

	return SyncAction.SKIP_NO_CHANGE;
}

async function sync(auth: OAuth2Client, remoteFiles: Map<string, DriveFile>, metadata: Map<string, FileMetadata>) {
	const resolvedLocalPath = resolvePath(LOCAL_SYNC_PATH);
	if (!resolvedLocalPath || !REMOTE_FOLDER_ID) {
		const errorMessage = "Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.";
		ui.logEvent("ERROR", errorMessage);
		logger.error(errorMessage);
		return;
	}
	await fs.mkdir(resolvedLocalPath, { recursive: true });

	ui.updateStatus("Active");
	ui.updateProgress({ currentActivity: "Starting sync cycle..." });
	logger.info("Starting sync cycle...");

	// --- Metadata Loading ---
	const metadataPath = path.join(resolvedLocalPath, METADATA_FILE_NAME);
	try {
		const data = await fs.readFile(metadataPath, "utf-8");
		metadata = new Map(JSON.parse(data));
		logger.info("Loaded sync metadata.");
	} catch (e: any) {
		if (e.code !== "ENOENT") logger.error(`Error loading sync metadata: ${e.message}`);
		else logger.warn("No sync metadata file found. Starting fresh.");
	}

	// --- Load config and prepare ignore patterns ---
	const config = await loadConfig();
	const ignorePatterns = (config.ignore || []).map((pattern) => new RegExp(pattern));

	// --- File Scanning ---
	ui.updateProgress({ currentActivity: "Scanning local and remote files..." });
	let [localFiles, currentRemoteFiles] = await Promise.all([
		getLocalFilesRecursive(resolvedLocalPath, ignorePatterns),
		listFilesRecursive(auth, REMOTE_FOLDER_ID, ignorePatterns),
	]);

	// Update the shared remoteFiles map with the latest scan
	remoteFiles.clear();
	currentRemoteFiles.forEach((file, filePath) => remoteFiles.set(filePath, file));

	// --- 1. Create Missing Remote Folders ---
	// This must be done first, and sequentially, to ensure parent folders exist.
	const remoteFolderPaths = new Set(
		Array.from(remoteFiles.values())
			.filter((f) => f.isDirectory)
			.map((f) => f.path)
	);
	const localFoldersToCreate = Array.from(localFiles.values())
		.filter((f) => f.isDirectory && !remoteFolderPaths.has(f.path))
		.sort((a, b) => a.path.split("/").length - b.path.split("/").length); // Sort by depth

	if (localFoldersToCreate.length > 0) {
		logger.info(`Creating ${localFoldersToCreate.length} missing remote folders...`);
		ui.logEvent("INFO", `Creating ${localFoldersToCreate.length} missing remote folders...`);
		for (const folder of localFoldersToCreate) {
			const parentPath = path.dirname(folder.path);
			const parentFolder = remoteFiles.get(parentPath);
			const parentFolderId = parentPath === "." ? REMOTE_FOLDER_ID : parentFolder?.id;
			if (parentFolderId) {
				try {
					const newFolder = await createFolder(auth, parentFolderId, path.basename(folder.path));
					// Add new folder to remoteFiles map to be found by children folders
					remoteFiles.set(folder.path, {
						id: newFolder.id,
						path: folder.path,
						name: path.basename(folder.path),
						isDirectory: true,
						modifiedTime: new Date().toISOString(), // Placeholder time
					});
				} catch (error: any) {
					logger.error(`Failed to create remote folder ${folder.path}. Error: ${error.message}`);
				}
			}
		}
		logger.info("Remote folder creation complete.");
	}

	// --- 2. Determine All Sync Tasks ---
	const allFilePaths = new Set([...localFiles.keys(), ...remoteFiles.keys()]);
	const tasks: SyncTask[] = [];

	for (const filePath of allFilePaths) {
		const localFile = localFiles.get(filePath);
		const remoteFile = remoteFiles.get(filePath);
		// We only sync files, not folders (folders were handled above)
		if (localFile?.isDirectory || remoteFile?.isDirectory) continue;

		const action = determineSyncAction(filePath, localFile, remoteFile, metadata);
		if (action !== SyncAction.SKIP_NO_CHANGE) {
			tasks.push({ action, filePath });
		}
	}

	// --- 3. Execute All Sync Tasks with Concurrency Limit ---
	if (tasks.length === 0) {
		logger.info("All files are up to date.");
		ui.logEvent("INFO", "All files are up to date.");
	} else {
		logger.info(`Executing ${tasks.length} sync tasks...`);
		ui.logEvent("INFO", `Executing ${tasks.length} sync tasks...`);

		const promises = tasks.map((task) =>
			retryOperation(async () => {
				const localFilePath = path.join(resolvedLocalPath, task.filePath);
				const remoteFile = remoteFiles.get(task.filePath);

				switch (task.action) {
					case SyncAction.DOWNLOAD_NEW:
					case SyncAction.DOWNLOAD_UPDATE:
						ui.logEvent("INFO", `[DOWNLOAD] Starting: ${task.filePath}`);
						await fs.mkdir(path.dirname(localFilePath), { recursive: true });
						await downloadFile(auth, remoteFile!.id, localFilePath);
						metadata.set(task.filePath, {
							remoteMd5Checksum: remoteFile!.md5Checksum,
						});
						ui.logEvent("SUCCESS", `[DOWNLOAD] Success: ${task.filePath}`);
						break;

					case SyncAction.UPLOAD_NEW:
					case SyncAction.UPLOAD_UPDATE:
					case SyncAction.UPLOAD_CONFLICT:
						ui.logEvent("INFO", `[UPLOAD] Starting (${SyncAction[task.action]}): ${task.filePath}`);
						const parentPath = path.dirname(task.filePath);
						const parentFolder = remoteFiles.get(parentPath);
						const parentFolderId = parentPath === "." ? REMOTE_FOLDER_ID : parentFolder?.id;

						if (!parentFolderId) {
							throw new Error(`Could not find remote parent folder for ${task.filePath}`);
						}

						const uploadedFile = await uploadOrUpdateFile(auth, localFilePath, {
							name: path.basename(task.filePath),
							folderId: parentFolderId,
							fileId: remoteFile?.id, // Will be undefined for new files, which is correct
						});
						metadata.set(task.filePath, {
							remoteMd5Checksum: uploadedFile.md5Checksum,
						});
						ui.logEvent("SUCCESS", `[UPLOAD] Success: ${task.filePath}`);
						break;

					case SyncAction.SKIP_IDENTICAL:
						ui.logEvent("INFO", `[SKIP] Identical content (MD5 match): ${task.filePath}`);
						// Update metadata to reflect current remote state
						metadata.set(task.filePath, {
							remoteMd5Checksum: remoteFile!.md5Checksum,
						});
						break;
				}
			}, `Sync task for ${task.filePath}`)
		);

		await Promise.all(promises);
	}

	// --- 4. Save Metadata ---
	try {
		await fs.writeFile(metadataPath, JSON.stringify(Array.from(metadata.entries())), "utf-8");
		logger.info("Sync metadata saved.");
	} catch (e: any) {
		logger.error(`Error saving sync metadata: ${e.message}`);
	}

	ui.updateStatus("Idle");
	ui.updateProgress({ currentActivity: "Sync cycle finished." });
	logger.info("Sync cycle finished.");
}

let syncTimeout: NodeJS.Timeout | null = null;

function watchLocalFiles(
	localPath: string,
	auth: OAuth2Client,
	remoteFiles: Map<string, DriveFile>,
	metadata: Map<string, FileMetadata>,
	ignorePatterns: RegExp[]
) {
	const watcher = chokidar.watch(localPath, {
		ignored: [/(^|[\\/])\.gdrive-sync-metadata\.json$/, ...ignorePatterns],
		persistent: true,
		ignoreInitial: true, // Don't trigger on initial scan
	});

	watcher.on("all", async (event, filePath) => {
		const relativePath = path.relative(localPath, filePath);
		logger.info(`Local file change detected: ${event} ${relativePath}`);
		ui.logEvent("INFO", `Local file change detected: ${event} ${relativePath}`);

		if (syncTimeout) {
			clearTimeout(syncTimeout);
		}
		syncTimeout = setTimeout(async () => {
			const localFilePath = path.join(localPath, relativePath);
			const remoteFile = remoteFiles.get(relativePath);

			try {
				switch (event) {
					case "add":
					case "change":
						// For add/change, upload or update the file
						const stats = await fs.stat(localFilePath);
						if (stats.size === 0) {
							ui.logEvent("INFO", `[UPLOAD] Skipping 0-byte file: ${relativePath}`);
							logger.info(`Skipping 0-byte file: ${relativePath}`);
							break;
						}
						ui.logEvent("INFO", `[UPLOAD] Starting: ${relativePath}`);
						const parentPath = path.dirname(relativePath);
						const parentFolder = remoteFiles.get(parentPath);
						const parentFolderId = parentPath === "." ? REMOTE_FOLDER_ID : parentFolder?.id;

						if (!parentFolderId) {
							throw new Error(`Could not find remote parent folder for ${relativePath}`);
						}

						const uploadedFile = await uploadOrUpdateFile(auth, localFilePath, {
							name: path.basename(relativePath),
							folderId: parentFolderId,
							fileId: remoteFile?.id, // Pass remoteFile.id to update existing file
						});
						metadata.set(relativePath, {
							remoteMd5Checksum: uploadedFile.md5Checksum,
						});
						ui.logEvent("SUCCESS", `[UPLOAD] Success: ${relativePath}`);
						break;
					case "unlink":
					case "unlinkDir": // Handle folder deletion
						// For unlink, delete the file/folder from remote
						if (remoteFile) {
							ui.logEvent("INFO", `[DELETE] Starting: ${relativePath}`);
							await deleteFile(auth, remoteFile.id);
							metadata.delete(relativePath);
							remoteFiles.delete(relativePath); // Remove from remoteFiles map
							ui.logEvent("SUCCESS", `[DELETE] Success: ${relativePath}`);
						} else {
							ui.logEvent(
								"INFO",
								`[DELETE] Remote file/folder not found for ${relativePath}, skipping.`
							);
						}
						break;
				}
			} catch (error: any) {
				const errorMessage = `[FAILED] Local change action for ${relativePath}. Error: ${error.message}`;
				ui.logEvent("ERROR", errorMessage);
				logger.error(errorMessage);
			}
			syncTimeout = null;
		}, WATCH_DEBOUNCE_DELAY);
	});

	logger.info(`Watching local path for changes: ${localPath}`);
	ui.logEvent("INFO", `Watching local path for changes: ${localPath}`);
}

// --- Continuous execution loop ---
(async () => {
	process.stdout.write("\x1b[?25l"); // Hide cursor
	process.on("exit", () => {
		process.stdout.write("\x1b[?25h"); // Show cursor on exit
		ui.stop();
	});

	ui.start();

	const resolvedLocalPath = resolvePath(LOCAL_SYNC_PATH);
	if (!resolvedLocalPath || !REMOTE_FOLDER_ID) {
		const errorMessage = "Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.";
		ui.logEvent("ERROR", errorMessage);
		logger.error(errorMessage);
		return;
	}

	await fs.mkdir(resolvedLocalPath, { recursive: true });

	const auth = await authorize();
	const remoteFiles: Map<string, DriveFile> = new Map();
	const metadata: Map<string, FileMetadata> = new Map();

	// --- Load config and prepare ignore patterns for watcher ---
	const config = await loadConfig();
	const ignorePatterns = (config.ignore || []).map((pattern) => new RegExp(pattern));

	// Initial sync
	await sync(auth, remoteFiles, metadata);

	// Start watching local files after initial sync
	watchLocalFiles(resolvedLocalPath, auth, remoteFiles, metadata, ignorePatterns);

	while (true) {
		try {
			// The sync function will now be triggered by the watcher or the interval
			// We keep the interval for periodic full syncs or if watcher fails
			await new Promise((resolve) => setTimeout(resolve, 60000)); // 60-second interval
			logger.info("Triggering periodic sync...");
			ui.logEvent("INFO", "Triggering periodic sync...");
			await sync(auth, remoteFiles, metadata);
		} catch (error: any) {
			const errorMessage = `An unexpected error occurred during the sync cycle: ${error.message || error}`;
			ui.logEvent("ERROR", errorMessage);
			logger.error(errorMessage);
			ui.updateStatus("Error");
		}
	}
})();
