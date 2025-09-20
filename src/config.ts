import { promises as fs } from "fs";
import * as path from "path";
import logger from "./services/logger";

// --- CONFIGURATION ---
export const LOCAL_SYNC_PATH = "~/GoogleDrive2";
export const REMOTE_FOLDER_ID = "root";
export const METADATA_FILE_NAME = ".az-gdrive-sync-metadata.json";
export const WATCH_DEBOUNCE_DELAY = 5000; // 5 seconds debounce for file changes
export const PERIODIC_SYNC_INTERVAL_MS = 1 * 60 * 1000; // 1 minute
export const LOG_DIR = "logs";
export const CONFIG_PATH = path.join(process.cwd(), "config.json");

export interface Config {
	ignore?: string[];
	LOCAL_SYNC_PATH?: string;
	REMOTE_FOLDER_ID?: string;
	METADATA_FILE_NAME?: string;
	WATCH_DEBOUNCE_DELAY?: number;
	PERIODIC_SYNC_INTERVAL_MS?: number;
	LOG_DIR?: string;
}

const DefaultConfig: Config = {
	LOCAL_SYNC_PATH: "~/GoogleDrive2",
	REMOTE_FOLDER_ID: "root",
	METADATA_FILE_NAME: ".az-gdrive-sync-metadata.json",
	WATCH_DEBOUNCE_DELAY: 5000,
	PERIODIC_SYNC_INTERVAL_MS: 1 * 60 * 1000, // 1 minute default
	LOG_DIR: "logs",
};

export async function loadConfig(): Promise<Config> {
	try {
		const content = await fs.readFile(CONFIG_PATH, "utf8");
		const loadedConfig: Config = JSON.parse(content);
		// Merge loaded config with defaults
		return { ...DefaultConfig, ...loadedConfig };
	} catch (err) {
		logger.warn("No config.json found or error reading config. Using default settings.");
		return DefaultConfig; // Return default config if file not found or error
	}
}
