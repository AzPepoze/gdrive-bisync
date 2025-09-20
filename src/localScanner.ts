import { promises as fs } from "fs";
import * as path from "path";
import { createHash } from "crypto";
import { createReadStream } from "fs";

export interface LocalFile {
	path: string;
	mtime: string;
	md5Checksum?: string;
	isDirectory: boolean;
}

export type LocalFileMap = Map<string, LocalFile>;

async function getFileMd5(filePath: string): Promise<string> {
	return new Promise((resolve, reject) => {
		const hash = createHash("md5");
		const stream = createReadStream(filePath);
		stream.on("data", (data) => hash.update(data));
		stream.on("end", () => resolve(hash.digest("hex")));
		stream.on("error", (err) => reject(err));
	});
}

export async function getLocalFilesRecursive(rootPath: string, ignorePatterns: RegExp[] = []): Promise<LocalFileMap> {
	const fileMap: LocalFileMap = new Map();

	async function traverse(currentDir: string, relativePath: string) {
		const entries = await fs.readdir(currentDir, { withFileTypes: true });
		for (const entry of entries) {
			const fullPath = path.join(currentDir, entry.name);
			const newRelativePath = path.join(relativePath, entry.name);

			// Check if the current entry (file or directory) should be ignored
			if (ignorePatterns.some(pattern => pattern.test(newRelativePath))) {
				console.log(`Ignoring ${newRelativePath} due to ignore pattern.`);
				// If it's a directory and matches an ignore pattern, we should not traverse into it.
				// If it's a file and matches, we just skip adding it.
				continue;
			}

			if (entry.isDirectory()) {
				fileMap.set(newRelativePath, {
					path: newRelativePath,
					mtime: (await fs.stat(fullPath)).mtime.toISOString(),
					isDirectory: true,
				});
				await traverse(fullPath, newRelativePath);
			} else if (entry.isFile()) {
				const stat = await fs.stat(fullPath);
				const md5 = await getFileMd5(fullPath); // MD5 calculation enabled
				fileMap.set(newRelativePath, {
					path: newRelativePath,
					mtime: stat.mtime.toISOString(),
					md5Checksum: md5, // MD5 checksum included
					isDirectory: false,
				});
			}
		}
	}

	await traverse(rootPath, "");
	return fileMap;
}
