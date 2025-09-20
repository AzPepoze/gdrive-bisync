import { google } from "googleapis";
import { promises as fs } from "fs";
import { OAuth2Client } from "google-auth-library";
import logger from "../services/logger";

const TOKEN_PATH = "token.json";

/**
 * Reads previously authorized credentials from the save file.
 * If token is not found, it will fail and instruct user to run the auth script.
 * @return {Promise<OAuth2Client>}
 */
export async function authorize(): Promise<OAuth2Client> {
	try {
		const content = await fs.readFile(TOKEN_PATH, "utf-8");
		const credentials = JSON.parse(content);
		const client = google.auth.fromJSON(credentials) as OAuth2Client;
		if (!client) {
			throw new Error("Invalid token file format.");
		}
		return client;
	} catch (err) {
		logger.error(`Authentication failed: Could not load token from '${TOKEN_PATH}'.`);
		logger.error("Please run \"pnpm authenticate\" to set up your credentials.");
		throw new Error("Authentication not configured.");
	}
}