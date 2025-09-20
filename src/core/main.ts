import { promises as fs } from "fs";
import { OAuth2Client } from "google-auth-library";

import { authorize } from "../api/googleAuth";
import { LOCAL_SYNC_PATH, REMOTE_FOLDER_ID, loadConfig } from "../config";
import logger from "../services/logger";
import { DriveFile, FileMetadata } from "../types";
import { ui } from "../ui/console";
import { resolvePath } from "../utils";
import { sync } from "./sync";
import { watchLocalFiles } from "./watcher";

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

	const config = await loadConfig();
	const ignorePatterns = (config.ignore || []).map((pattern) => new RegExp(pattern));

	// Initial sync
	await sync(auth, remoteFiles, metadata);

	// Start watching local files after initial sync
	watchLocalFiles(resolvedLocalPath, auth, remoteFiles, metadata, ignorePatterns);

	while (true) {
		try {
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
