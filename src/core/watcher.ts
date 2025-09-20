import * as path from "path";
import * as chokidar from "chokidar";
import { promises as fs } from "fs";
import { OAuth2Client } from "google-auth-library";

import { deleteFile, uploadOrUpdateFile } from "../api/driveApi";
import { WATCH_DEBOUNCE_DELAY, REMOTE_FOLDER_ID } from "../config";
import logger from "../services/logger";
import { DriveFile, FileMetadata } from "../types";
import { ui } from "../ui/console";

let syncTimeout: NodeJS.Timeout | null = null;

export function watchLocalFiles(
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
								fileId: remoteFile?.id,
							});
							metadata.set(relativePath, {
								remoteMd5Checksum: uploadedFile.md5Checksum,
							});
							ui.logEvent("SUCCESS", `[UPLOAD] Success: ${relativePath}`);
							break;
					case "unlink":
					case "unlinkDir":
						if (remoteFile) {
							ui.logEvent("INFO", `[DELETE] Starting: ${relativePath}`);
							await deleteFile(auth, remoteFile.id);
							metadata.delete(relativePath);
							remoteFiles.delete(relativePath);
							ui.logEvent("SUCCESS", `[DELETE] Success: ${relativePath}`);
						} else {
							ui.logEvent("INFO", `[DELETE] Remote file/folder not found for ${relativePath}, skipping.`);
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
