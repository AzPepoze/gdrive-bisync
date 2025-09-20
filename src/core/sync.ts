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
import { Config } from "../config";
import { retryOperation } from "../utils";

export async function sync(
	auth: OAuth2Client,
	remoteFiles: Map<string, DriveFile>,
	metadata: Map<string, FileMetadata>,
	config: Config
) {
	const resolvedLocalPath = resolvePath(config.LOCAL_SYNC_PATH!);
	if (!resolvedLocalPath || !config.REMOTE_FOLDER_ID) {
		const errorMessage = "Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.";
		logger.error(errorMessage);
		return;
	}
	await fs.mkdir(resolvedLocalPath, { recursive: true });

	logger.info("Starting sync cycle...");
	ui.updateStatus("Starting sync cycle...");

	// --- Metadata Loading ---
	const metadataPath = path.join(resolvedLocalPath, config.METADATA_FILE_NAME!);
	try {
		const data = await fs.readFile(metadataPath, "utf-8");
		metadata = new Map(JSON.parse(data));
		logger.info("Loaded sync metadata.");
	} catch (e: any) {
		if (e.code !== "ENOENT") logger.error(`Error loading sync metadata: ${e.message}`);
		else logger.warn("No sync metadata file found. Starting fresh.");
	}

	// --- Load config and prepare ignore patterns ---
	const ignorePatterns = (config.ignore || []).map((pattern) => new RegExp(pattern));

	// --- File Scanning ---
	let [localFiles, currentRemoteFiles] = await Promise.all([
		getLocalFilesRecursive(resolvedLocalPath, ignorePatterns, (path) =>
			ui.updateStatus(`Scanning local: /${path}`)
		),
		listFilesRecursive(auth, config.REMOTE_FOLDER_ID!, ignorePatterns, (path) =>
			ui.updateStatus(`Scanning remote: /${path}`)
		),
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
		ui.updateStatus(`Creating ${localFoldersToCreate.length} remote folders...`);
		for (const folder of localFoldersToCreate) {
			const parentPath = path.dirname(folder.path);
			const parentFolder = remoteFiles.get(parentPath);
			const parentFolderId = parentPath === "." ? config.REMOTE_FOLDER_ID! : parentFolder?.id;
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
		if (action !== SyncAction.SKIP_NO_CHANGE && action !== SyncAction.SKIP_IDENTICAL) {
			tasks.push({ action, filePath });
		}
	}

	// --- 3. Execute All Sync Tasks ---
	if (tasks.length === 0) {
		logger.info("All files are up to date.");
	} else {
		logger.info(`Executing ${tasks.length} sync tasks...`);
		let completedTasks = 0;

		const taskPromises = tasks.map(async (task) => {
			await retryOperation(async () => {
				const localFilePath = path.join(resolvedLocalPath, task.filePath);
				const remoteFile = remoteFiles.get(task.filePath);
				const actionString = SyncAction[task.action];

				ui.updateStatus(`[${completedTasks + 1}/${tasks.length}] ${actionString}: ${task.filePath}`);

				switch (task.action) {
					case SyncAction.DOWNLOAD_NEW:
					case SyncAction.DOWNLOAD_UPDATE:
						await fs.mkdir(path.dirname(localFilePath), { recursive: true });
						await downloadFile(auth, remoteFile!.id, localFilePath);
						metadata.set(task.filePath, { remoteMd5Checksum: remoteFile!.md5Checksum });
						break;

					case SyncAction.UPLOAD_NEW:
					case SyncAction.UPLOAD_UPDATE:
					case SyncAction.UPLOAD_CONFLICT:
						const parentPath = path.dirname(task.filePath);
						const parentFolder = remoteFiles.get(parentPath);
						const parentFolderId = parentPath === "." ? config.REMOTE_FOLDER_ID! : parentFolder?.id;

						if (!parentFolderId) {
							throw new Error(`Could not find remote parent folder for ${task.filePath}`);
						}

						const uploadedFile = await uploadOrUpdateFile(auth, localFilePath, {
							name: path.basename(task.filePath),
							folderId: parentFolderId,
							fileId: remoteFile?.id,
						});
						metadata.set(task.filePath, { remoteMd5Checksum: uploadedFile.md5Checksum });
						break;
				}
			}, `Sync task for ${task.filePath}`);
			completedTasks++;
		});

		await Promise.all(taskPromises);
	}

	// --- 4. Save Metadata ---
	try {
		await fs.writeFile(metadataPath, JSON.stringify(Array.from(metadata.entries())), "utf-8");
		logger.info("Sync metadata saved.");
	} catch (e: any) {
		logger.error(`Error saving sync metadata: ${e.message}`);
	}

	logger.info("Sync cycle finished.");
	ui.startIdleCountdown(config.PERIODIC_SYNC_INTERVAL_MS!);
}
