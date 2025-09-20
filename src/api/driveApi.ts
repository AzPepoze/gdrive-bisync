import { google } from "googleapis";
import { OAuth2Client } from "google-auth-library";
import { createReadStream, createWriteStream } from "fs";
import * as path from "path";
import logger from "../services/logger";
import { DriveFileMap } from "../types";
import { retryOperation } from "../utils";

export async function listFilesRecursive(
	auth: OAuth2Client,
	folderId: string,
	ignorePatterns: RegExp[] = [],
	onProgress?: (path: string) => void
): Promise<DriveFileMap> {
	const drive = google.drive({ version: "v3", auth });
	const fileMap: DriveFileMap = new Map();

	async function traverse(currentFolderId: string, currentPath: string) {
		onProgress?.(currentPath || "/");
		let pageToken: string | undefined = undefined;
		const folderPromises: Promise<void>[] = []; // Array to hold promises for parallel folder traversals

		do {
			const res = await retryOperation(
				async () =>
					drive.files.list({
						q: `'${currentFolderId}' in parents and trashed = false`,
						fields: "nextPageToken, files(id, name, mimeType, modifiedTime, md5Checksum)",
						pageToken: pageToken as string,
						pageSize: 1000,
					}),
				`List files in folder ${currentFolderId} : ${currentPath}`
			);

			if (res.data.files) {
				for (const file of res.data.files) {
					const filePath = path.join(currentPath, file.name!); // Use file.name! as it's guaranteed to exist here
					const isDirectory = file.mimeType === "application/vnd.google-apps.folder";

					// Check if the current entry (file or directory) should be ignored
					if (ignorePatterns.some((pattern) => pattern.test(filePath))) {
						logger.debug(`Ignoring Drive file/folder: ${filePath} due to ignore pattern.`);
						continue;
					}

					fileMap.set(filePath, {
						id: file.id!,
						name: file.name!,
						path: filePath,
						modifiedTime: file.modifiedTime!,
						md5Checksum: file.md5Checksum || undefined,
						isDirectory,
					});

					if (isDirectory) {
						folderPromises.push(traverse(file.id!, filePath)); // Add to promises array
					}
				}
			}
			pageToken = res.data.nextPageToken || undefined;
		} while (pageToken);
		await Promise.all(folderPromises); // Wait for all parallel folder traversals to complete
	}

	await traverse(folderId, "");
	return fileMap;
}

export async function listFilesRecursiveParallel(auth: OAuth2Client, folderIds: string[]): Promise<DriveFileMap> {
	const allFileMaps = await Promise.all(folderIds.map((folderId) => listFilesRecursive(auth, folderId)));

	const combinedFileMap: DriveFileMap = new Map();
	allFileMaps.forEach((fileMap) => {
		fileMap.forEach((file, filePath) => {
			combinedFileMap.set(filePath, file);
		});
	});

	return combinedFileMap;
}
export async function downloadFile(auth: OAuth2Client, fileId: string, destinationPath: string): Promise<void> {
	const drive = google.drive({ version: "v3", auth });
	const dest = createWriteStream(destinationPath);

	const res = await drive.files.get({ fileId: fileId, alt: "media" }, { responseType: "stream" });

	return new Promise((resolve, reject) => {
		(res.data as any)
			.on("end", () => resolve())
			.on("error", (err: any) => reject(err))
			.pipe(dest);
	});
}

export async function downloadFilesParallel(
	auth: OAuth2Client,
	filesToDownload: Array<{ fileId: string; destinationPath: string }>
): Promise<void> {
	const downloadPromises = filesToDownload.map(async (fileInfo) => {
		try {
			await downloadFile(auth, fileInfo.fileId, fileInfo.destinationPath);
			logger.info(`Finished download for file ID: ${fileInfo.fileId}`);
		} catch (error) {
			console.error(`Error downloading file ID: ${fileInfo.fileId}`, error);
			throw error; // Re-throw to indicate failure in Promise.all
		}
	});

	await Promise.all(downloadPromises);
	console.log("All parallel downloads completed.");
}

export async function uploadOrUpdateFile(
	auth: OAuth2Client,
	localPath: string,
	remoteInfo: { name: string; folderId: string; fileId?: string }
): Promise<any> {
	const drive = google.drive({ version: "v3", auth });
	const media = {
		body: createReadStream(localPath),
	};

	if (remoteInfo.fileId) {
		// Update existing file
		const res = await drive.files.update({
			fileId: remoteInfo.fileId,
			media: media,
			fields: "id, name, modifiedTime, md5Checksum",
		});
		return res.data;
	} else {
		// Create new file
		const res = await drive.files.create({
			requestBody: {
				name: remoteInfo.name,
				parents: [remoteInfo.folderId],
			},
			media: media,
			fields: "id, name, modifiedTime, md5Checksum",
		});
		return res.data;
	}
}

export async function createFolder(
	auth: OAuth2Client,
	parentFolderId: string,
	folderName: string
): Promise<{ id: string }> {
	const drive = google.drive({ version: "v3", auth });
	const res = await drive.files.create({
		requestBody: {
			name: folderName,
			mimeType: "application/vnd.google-apps.folder",
			parents: [parentFolderId],
		},
		fields: "id",
	});
	return { id: res.data.id! };
}

export async function deleteFile(auth: OAuth2Client, fileId: string): Promise<void> {
	const drive = google.drive({ version: "v3", auth });
	await drive.files.delete({ fileId: fileId });
}
