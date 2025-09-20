import { google } from "googleapis";
import { OAuth2Client } from "google-auth-library";
import { createReadStream, createWriteStream } from "fs";
import * as path from "path";

export interface DriveFile {
	id: string;
	name: string;
	path: string;
	modifiedTime: string;
	md5Checksum?: string;
	isDirectory: boolean;
}

export type DriveFileMap = Map<string, DriveFile>;

export async function listFilesRecursive(auth: OAuth2Client, folderId: string): Promise<DriveFileMap> {
	const drive = google.drive({ version: "v3", auth });
	const fileMap: DriveFileMap = new Map();

	async function traverse(currentFolderId: string, currentPath: string) {
		let pageToken: string | undefined = undefined;
		console.log(`Scanning folder: ${currentPath || "root"}`); // Log the current folder
		const folderPromises: Promise<void>[] = []; // Array to hold promises for parallel folder traversals

		do {
			const res: any = await drive.files.list({
				q: `'${currentFolderId}' in parents and trashed = false`,
				fields: "nextPageToken, files(id, name, mimeType, modifiedTime, md5Checksum)",
				pageToken: pageToken as string,
				pageSize: 1000,
			});

			if (res.data.files) {
				for (const file of res.data.files) {
					const filePath = path.join(currentPath, file.name!);
					const isDirectory = file.mimeType === "application/vnd.google-apps.folder";

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
		console.log(`Starting download for file ID: ${fileInfo.fileId} to ${fileInfo.destinationPath}`);
		try {
			await downloadFile(auth, fileInfo.fileId, fileInfo.destinationPath);
			console.log(`Finished download for file ID: ${fileInfo.fileId}`);
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
