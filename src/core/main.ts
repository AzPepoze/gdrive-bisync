import { promises as fs } from "fs";

import { authorize } from "../api/googleAuth";
import { loadConfig } from "../config";
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
		logger.end();
	});

	ui.start();

	const config = await loadConfig();

	const resolvedLocalPath = resolvePath(config.LOCAL_SYNC_PATH!);
	if (!resolvedLocalPath || !config.REMOTE_FOLDER_ID) {
		const errorMessage = "Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.";
		logger.error(errorMessage);
		return;
	}

	await fs.mkdir(resolvedLocalPath, { recursive: true });

	try {
		const auth = await authorize();
		const remoteFiles: Map<string, DriveFile> = new Map();
		const metadata: Map<string, FileMetadata> = new Map();

		// Initial sync
		await sync(auth, remoteFiles, metadata, config);

		// Start watching local files after initial sync
		watchLocalFiles(resolvedLocalPath, auth, remoteFiles, metadata, config);

		// Set up periodic sync
		setInterval(async () => {
			logger.info("Triggering periodic sync...");
			await sync(auth, remoteFiles, metadata, config);
		}, config.PERIODIC_SYNC_INTERVAL_MS!);
	} catch (error: any) {
		// This will catch the authentication error if authorize() fails
		ui.stop(); // Stop the spinner on auth failure
		// The error message is already logged by authorize()
	}
})();
