import { google } from "googleapis";
import { promises as fs } from "fs";
import * as http from "http";
import { URL } from "url";
import { OAuth2Client } from "google-auth-library";
import logger from "./services/logger";
import { ui } from "./ui/console";
import { exit } from "process";

// Dynamically import open
const open = async (url: string) => {
	const openModule = await import("open");
	return openModule.default(url);
};

const SCOPES = ["https://www.googleapis.com/auth/drive"];
const TOKEN_PATH = "config/token.json";
const CREDENTIALS_PATH = "config/credentials.json";

/**
 * Serializes credentials to a file compatible with GoogleAUth.fromJSON.
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
 * Guides the user through a seamless OAuth2 flow using a local server.
 */
async function setupAuthentication() {
	logger.info("--- Google Drive Authentication Setup ---");

	let credentials;
	try {
		const content = await fs.readFile(CREDENTIALS_PATH, "utf-8");
		credentials = JSON.parse(content);
	} catch (err) {
		logger.error(`Error: '${CREDENTIALS_PATH}' not found.`);
		logger.info("Please follow these steps to get your credentials file:");
		console.log(
			`
1. Go to the Google Cloud Console: https://console.cloud.google.com/
2. Create a new project or select an existing one.
3. In the library, search for and enable the "Google Drive API".
4. Go to "Credentials" -> "Create Credentials" -> "OAuth client ID".
5. Select "Desktop app" as the application type.
6. Download the JSON file provided after creation.
7. IMPORTANT: Rename the downloaded file to "credentials.json" and place it in the "config" directory of this project.
`
		);
		return;
	}

	const { client_secret, client_id, redirect_uris } = credentials.installed;
	const oAuth2Client = new google.auth.OAuth2(client_id, client_secret, redirect_uris[0]);

	try {
		ui.updateStatus("Waiting for user authentication...");
		const code = await listenForCode(oAuth2Client, redirect_uris[0]);
		const { tokens } = await oAuth2Client.getToken(code);
		oAuth2Client.setCredentials(tokens);
		await saveCredentials(oAuth2Client);
		logger.info(`✅ Authentication successful! Token saved to '${TOKEN_PATH}'`);
	} catch (err: any) {
		logger.error("Authentication process failed.");
		logger.error(err.message || err);
	}

	ui.stop();
	exit(0);
}

function listenForCode(oAuth2Client: OAuth2Client, redirectUri: string): Promise<string> {
	return new Promise((resolve, reject) => {
		let port: number;
		try {
			const parsedPort = new URL(redirectUri).port;
			if (!parsedPort) {
				throw new Error("Redirect URI in config/credentials.json is missing a port number.");
			}
			port = parseInt(parsedPort, 10);
		} catch (e) {
			logger.error(`Invalid redirect URI in config/credentials.json: "${redirectUri}"`);
			logger.info("The redirect URI must be a valid 'http://localhost:[PORT]' URL.");
			console.log(
				`
				Please fix your credentials by following these steps:
				1. Go to the Google Cloud Console: https://console.cloud.google.com/apis/credentials
				2. Select your OAuth 2.0 Client ID for Desktop app.
				3. Under "Authorized redirect URIs", click "ADD URI".
				4. Enter "http://localhost:3000" (or another port of your choice).
				5. Save the changes in the Google Cloud Console.
				6. In your local "config/credentials.json" file, ensure the "redirect_uris" array contains the exact same URI (e.g., ["http://localhost:3000"]).
				`
			);
			return reject(new Error("Invalid or incomplete redirect URI."));
		}

		const server = http.createServer((req, res) => {
			try {
				const requestUrl = new URL(req.url!, `http://localhost:${port}`);
				const authCode = requestUrl.searchParams.get("code");

				if (authCode) {
					res.writeHead(200, { "Content-Type": "text/html" });
					res.end(
						"<h1>Authentication successful!</h1><p>You can now close this browser tab and return to the terminal.</p>"
					);
					server.close();
					resolve(authCode);
				} else {
					const error = requestUrl.searchParams.get("error") || "Unknown error";
					res.writeHead(400, { "Content-Type": "text/html" });
					res.end(`<h1>Authentication failed</h1><p>Error: ${error}. Please try again.</p>`);
					server.close();
					reject(new Error(`Authentication failed with error: ${error}`));
				}
			} catch (e: any) {
				reject(e);
			}
		});

		server.on("error", reject);

		server.listen(port, async () => {
			const authUrl = oAuth2Client.generateAuthUrl({
				access_type: "offline",
				scope: SCOPES,
			});

			console.log("Your browser should open automatically to complete the authentication.");
			console.log(`If it doesn't, please open this URL: ${authUrl}`);

			try {
				if (typeof authUrl !== "string" || !authUrl) {
					throw new Error("Generated authUrl is invalid.");
				}
				await open(authUrl);
			} catch (error) {
				logger.error("Failed to automatically open browser. Please open the URL manually.");
			}
		});
	});
}

setupAuthentication();
