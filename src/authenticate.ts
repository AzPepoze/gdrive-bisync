import { google } from "googleapis";
import { promises as fs } from "fs";
import * as readline from "readline";
import { OAuth2Client } from "google-auth-library";
import logger from "./logger";

// If modifying these scopes, delete token.json.
const SCOPES = ["https://www.googleapis.com/auth/drive"];
// The file token.json stores the user's access and refresh tokens, and is
// created automatically when the authorization flow completes for the first
// time.
const TOKEN_PATH = "token.json";
const CREDENTIALS_PATH = "credentials.json";

/**
 * Reads previously authorized credentials from the save file.
 *
 * @return {Promise<OAuth2Client|null>}
 */
async function loadSavedCredentialsIfExist(): Promise<OAuth2Client | null> {
	try {
		const content = await fs.readFile(TOKEN_PATH, "utf-8");
		const credentials = JSON.parse(content);
		return google.auth.fromJSON(credentials) as OAuth2Client;
	} catch (err) {
		return null;
	}
}

/**
 * Serializes credentials to a file compatible with GoogleAUth.fromJSON.
 *
 * @param {OAuth2Client} client
 * @return {Promise<void>}
 */
async function saveCredentials(client: OAuth2Client): Promise<void> {
	const content = await fs.readFile(CREDENTIALS_PATH, "utf-8");
	const keys = JSON.parse(content);
	const key = keys.installed || keys.web;
	const payload = JSON.stringify({
		type: "authorized_user",
		client_id: key.client_id,
		client_secret: key.client_secret,
		refresh_token: client.credentials.refresh_token,
	});
	await fs.writeFile(TOKEN_PATH, payload);
}

/**
 * Load or request or authorization to call APIs.
 *
 */
export async function authorize(): Promise<OAuth2Client> {
	// Exported authorize function
	let client = await loadSavedCredentialsIfExist();
	if (client) {
		return client;
	}

	const content = await fs.readFile(CREDENTIALS_PATH, "utf-8");
	const { client_secret, client_id, redirect_uris } = JSON.parse(content).installed;
	const oAuth2Client = new google.auth.OAuth2(client_id, client_secret, redirect_uris[0]);

	const rl = readline.createInterface({
		input: process.stdin,
		output: process.stdout,
	});

	const authUrl = oAuth2Client.generateAuthUrl({
		access_type: "offline",
		scope: SCOPES,
	});

	logger.info(`Authorize this app by visiting this url: ${authUrl}`);

	const code = await new Promise<string>((resolve) => {
		rl.question("Enter the code from that page here: ", (code) => {
			rl.close();
			resolve(code);
		});
	});

	try {
		const { tokens } = await oAuth2Client.getToken(code);
		oAuth2Client.setCredentials(tokens);
		await saveCredentials(oAuth2Client);
		logger.info(`Token stored to ${TOKEN_PATH}`);
		return oAuth2Client;
	} catch (err) {
		logger.error(`Error retrieving access token: ${err}`);
		throw err;
	}
}
