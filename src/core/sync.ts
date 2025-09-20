import { promises as fs } from "fs";
import * as path from "path";
import { OAuth2Client } from "google-auth-library";

import logger from "../services/logger";
import { getLocalFilesRecursive } from "../services/localScanner";
import { createFolder, downloadFile, listFilesRecursive, uploadOrUpdateFile } from "../api/driveApi";
import { ui } from "../ui/console";
import { resolvePath } from "../utils";
import { determineSyncAction } from "./decision";
import { DriveFile, FileMetadata, SyncAction, SyncTask } from "../types";
import { LOCAL_SYNC_PATH, METADATA_FILE_NAME, REMOTE_FOLDER_ID, loadConfig } from "../config";
import { retryOperation } from "../utils";

export async function sync(auth: OAuth2Client, remoteFiles: Map<string, DriveFile>, metadata: Map<string, FileMetadata>) {
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
	const remoteFolderPaths = new Set(
		Array.from(remoteFiles.values())
			.filter((f) => f.isDirectory)
			.map((f) => f.path)
	);
	const localFoldersToCreate = Array.from(localFiles.values())
		.filter((f) => f.isDirectory && !remoteFolderPaths.has(f.path))
		.sort((a, b) => a.path.split("/").length - b.path.split("/").length);

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
					remoteFiles.set(folder.path, {
						id: newFolder.id,
						path: folder.path,
						name: path.basename(folder.path),
						isDirectory: true,
						modifiedTime: new Date().toISOString(),
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
							fileId: remoteFile?.id,
						});
						metadata.set(task.filePath, {
							remoteMd5Checksum: uploadedFile.md5Checksum,
						});
						ui.logEvent("SUCCESS", `[UPLOAD] Success: ${task.filePath}`);
						break;

					case SyncAction.SKIP_IDENTICAL:
						ui.logEvent("INFO", `[SKIP] Identical content (MD5 match): ${task.filePath}`);
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
