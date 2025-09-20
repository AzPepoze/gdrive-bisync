import * as path from "path";
import * as chokidar from "chokidar";
import { promises as fs } from "fs";
import { OAuth2Client } from "google-auth-library";

import { deleteFile, uploadOrUpdateFile } from "../api/driveApi";
import { DriveFile, FileMetadata } from "../types";
import { Config } from "../config";
import { ui } from "../ui/console";
import logger from "../services/logger";

let syncTimeout: { [key: string]: NodeJS.Timeout } = {};

export function watchLocalFiles(
	localPath: string,
	auth: OAuth2Client,
	remoteFiles: Map<string, DriveFile>,
	metadata: Map<string, FileMetadata>,
	config: Config
) {
	const watcher = chokidar.watch(localPath, {
		ignored: [
			/(^|[\\/])\.az-gdrive-sync-metadata\.json$/,
			...(config.ignore || []).map((pattern) => new RegExp(pattern)),
		],
		persistent: true,
		ignoreInitial: true, // Don't trigger on initial scan
	});

	watcher.on("all", async (event, filePath) => {
		const relativePath = path.relative(localPath, filePath);
		logger.info(`Local file change detected: ${event} ${relativePath}`);

		ui.stopIdleCountdown();

		if (syncTimeout[relativePath]) {
			clearTimeout(syncTimeout[relativePath]);
		}

		syncTimeout[relativePath] = setTimeout(async () => {
			const localFilePath = path.join(localPath, relativePath);
			const remoteFile = remoteFiles.get(relativePath);

			try {
				ui.updateStatus(`Processing change: ${event} ${relativePath}`);
				switch (event) {
					case "add":
					case "change":
						const stats = await fs.stat(localFilePath);
						if (stats.size === 0) {
							logger.warn(`[UPLOAD] Skipping 0-byte file: ${relativePath}`);
							break;
						}
						ui.logEvent("INFO", `Uploading: ${relativePath}`);
						const parentPath = path.dirname(relativePath);
						const parentFolder = remoteFiles.get(parentPath);
						const parentFolderId = parentPath === "." ? config.REMOTE_FOLDER_ID! : parentFolder?.id;

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
						ui.logEvent("SUCCESS", `Uploaded: ${relativePath}`);
						break;
					case "unlink":
					case "unlinkDir":
						if (remoteFile) {
							ui.logEvent("INFO", `Deleting: ${relativePath}`);
							await deleteFile(auth, remoteFile.id);
							metadata.delete(relativePath);
							remoteFiles.delete(relativePath);
							ui.logEvent("SUCCESS", `Deleted: ${relativePath}`);
						} else {
							logger.warn(`[DELETE] Remote file/folder not found for ${relativePath}, skipping.`);
						}
						break;
				}
				ui.startIdleCountdown(config.PERIODIC_SYNC_INTERVAL_MS!);
			} catch (error: any) {
				const errorMessage = `[FAILED] Local change action for ${relativePath}. Error: ${error.message}`;
				logger.error(errorMessage);
				ui.updateStatus("Error processing change. Check logs.");
			}
			delete syncTimeout[relativePath];
		}, config.WATCH_DEBOUNCE_DELAY!);
	});

	ui.logEvent("INFO", `Watching for local changes in: ${localPath}`);
}
