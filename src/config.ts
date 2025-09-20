import { promises as fs } from "fs";
import * as path from "path";
import logger from "./services/logger";

// --- CONFIGURATION ---
export const LOCAL_SYNC_PATH = "~/GoogleDrive2";
export const REMOTE_FOLDER_ID = "root";
export const METADATA_FILE_NAME = ".az-gdrive-sync-metadata.json";
export const WATCH_DEBOUNCE_DELAY = 5000; // 5 seconds debounce for file changes
export const PERIODIC_SYNC_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes
export const CONFIG_PATH = path.join(process.cwd(), "config.json");

export interface Config {
	ignore?: string[];
}

export async function loadConfig(): Promise<Config> {
	try {
		const content = await fs.readFile(CONFIG_PATH, "utf8");
		return JSON.parse(content);
	} catch (err) {
		logger.warn("No config.json found or error reading config. Using default settings.");
		return {};
	}
}
