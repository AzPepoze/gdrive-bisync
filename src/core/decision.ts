import { DriveFile, FileMetadata, LocalFile, SyncAction } from "../types";

/**
 * Determines the required sync action for a single file.
 */
export function determineSyncAction(
	filePath: string,
	localFile: LocalFile | undefined,
	remoteFile: DriveFile | undefined,
	metadata: Map<string, FileMetadata>
): SyncAction {
	const lastSyncedInfo = metadata.get(filePath);

	if (localFile && !remoteFile) {
		return SyncAction.UPLOAD_NEW;
	}

	if (!localFile && remoteFile) {
		return SyncAction.DOWNLOAD_NEW;
	}

	if (localFile && remoteFile) {
		// If MD5 checksums are available and match, files are identical.
		if (localFile.md5Checksum && remoteFile.md5Checksum && localFile.md5Checksum === remoteFile.md5Checksum) {
			return SyncAction.SKIP_IDENTICAL;
		}

		const lastSyncedRemoteMd5 = lastSyncedInfo?.remoteMd5Checksum;

		if (lastSyncedRemoteMd5 === remoteFile.md5Checksum) {
			// Remote file has not changed since last sync. Local is newer or different.
			return SyncAction.UPLOAD_UPDATE;
		} else {
			// Remote file has changed since last sync.
			const localTime = new Date(localFile.mtime).getTime();
			const remoteTime = new Date(remoteFile.modifiedTime).getTime();
			const buffer = 2000; // 2-second buffer for timestamp comparison

			if (remoteTime > localTime + buffer) {
				return SyncAction.DOWNLOAD_UPDATE;
			} else {
				// Local is newer or we can't decide based on time, treat as conflict for now.
				return SyncAction.UPLOAD_CONFLICT; // Local version wins in this implementation
			}
		}
	}

	return SyncAction.SKIP_NO_CHANGE;
}
