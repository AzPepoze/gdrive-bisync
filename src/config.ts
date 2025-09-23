import { promises as fs } from "fs";
import * as path from "path";
import logger from "./services/logger";

export const CONFIG_PATH = path.join(process.cwd(), "config/config.json");

export interface Config {
	ignore?: string[];
	LOCAL_SYNC_PATH?: string;
	REMOTE_FOLDER_ID?: string;
	METADATA_FILE_NAME?: string;
	WATCH_DEBOUNCE_DELAY?: number;
	PERIODIC_SYNC_INTERVAL_MS?: number;
}

export const LOG_DIR = "logs";

const DefaultConfig: Config = {
	LOCAL_SYNC_PATH: "~/GoogleDrive",
	REMOTE_FOLDER_ID: "root",
	METADATA_FILE_NAME: ".gdrive-bisync-metadata.json",
	WATCH_DEBOUNCE_DELAY: 5000,
	PERIODIC_SYNC_INTERVAL_MS: 1 * 60 * 1000, // 1 minute default
};

export async function loadConfig(): Promise<Config> {
	try {
		const content = await fs.readFile(CONFIG_PATH, "utf8");
		const loadedConfig: Config = JSON.parse(content);
		const config = { ...DefaultConfig, ...loadedConfig };

		if (!config.ignore) {
			config.ignore = [];
		}

		if (config.METADATA_FILE_NAME) {
			config.ignore.push(`^${config.METADATA_FILE_NAME.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`);
		}

		return config;
	} catch (err) {
		logger.warn("No config/config.json found or error reading config. Using default settings.");
		return DefaultConfig;
	}
}
