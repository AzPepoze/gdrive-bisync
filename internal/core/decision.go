package core

import (
	"time"

	"gdrive-bisync/internal/types"
)

func DetermineSyncAction(
	filePath string,
	localFile *types.LocalFile,
	remoteFile *types.DriveFile,
	metadata map[string]*types.FileMetadata,
) types.SyncAction {
	lastSyncedInfo, hasSynced := metadata[filePath]

	// --- Deletion Logic ---
	if hasSynced {
		if localFile != nil && remoteFile == nil {
			// File exists locally and was synced before, but now missing remotely
			return types.ActionDeleteLocal
		}
		if localFile == nil && remoteFile != nil {
			// File exists remotely and was synced before, but now missing locally
			return types.ActionDeleteRemote
		}
	}

	if localFile != nil && remoteFile == nil {
		return types.ActionUploadNew
	}

	if localFile == nil && remoteFile != nil {
		return types.ActionDownloadNew
	}

	if localFile != nil && remoteFile != nil {
		// If MD5 checksums are available and match, files are identical.
		if localFile.MD5Checksum != "" && remoteFile.MD5Checksum != "" && localFile.MD5Checksum == remoteFile.MD5Checksum {
			return types.ActionSkipIdentical
		}

		lastSyncedRemoteMd5 := ""
		if hasSynced {
			lastSyncedRemoteMd5 = lastSyncedInfo.RemoteMD5Checksum
		}

		if lastSyncedRemoteMd5 == remoteFile.MD5Checksum {
			// Remote file has not changed since last sync. Local is newer or different.
			return types.ActionUploadUpdate
		} else {
			// Remote file has changed since last sync.
			buffer := 2 * time.Second
			if remoteFile.ModifiedTime.After(localFile.ModTime.Add(buffer)) {
				return types.ActionDownloadUpdate
			} else {
				// Local is newer or we can't decide based on time, treat as conflict.
				return types.ActionUploadConflict // Local version wins in this implementation
			}
		}
	}

	return types.ActionSkipNoChange
}
