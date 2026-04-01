package core

import (
	"time"

	"gdrive-bisync/internal/types"
)

func localChangedSinceSync(localFile *types.LocalFile, lastSyncedInfo *types.FileMetadata) bool {
	if localFile == nil || lastSyncedInfo == nil {
		return false
	}

	if localFile.MD5Checksum != "" && lastSyncedInfo.LocalMD5Checksum != "" {
		return localFile.MD5Checksum != lastSyncedInfo.LocalMD5Checksum
	}

	if !lastSyncedInfo.LocalModTime.IsZero() {
		return !localFile.ModTime.Equal(lastSyncedInfo.LocalModTime)
	}

	return false
}

func remoteChangedSinceSync(remoteFile *types.DriveFile, lastSyncedInfo *types.FileMetadata) bool {
	if remoteFile == nil || lastSyncedInfo == nil {
		return false
	}

	if remoteFile.MD5Checksum != "" && lastSyncedInfo.RemoteMD5Checksum != "" {
		return remoteFile.MD5Checksum != lastSyncedInfo.RemoteMD5Checksum
	}

	return false
}

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
			if localChangedSinceSync(localFile, lastSyncedInfo) {
				// Local changed while remote disappeared. Protect local changes.
				return types.ActionUploadNew
			}
			// File exists locally and was synced before, but now missing remotely and local is unchanged.
			return types.ActionDeleteLocal
		}
		if localFile == nil && remoteFile != nil {
			if remoteChangedSinceSync(remoteFile, lastSyncedInfo) {
				// Remote changed since last sync. Avoid destructive delete and restore locally.
				return types.ActionDownloadUpdate
			}
			// File exists remotely and was synced before, but now missing locally and remote is unchanged.
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

		lastSyncedRemoteMD5 := ""
		if hasSynced {
			lastSyncedRemoteMD5 = lastSyncedInfo.RemoteMD5Checksum
		}

		if hasSynced {
			remoteChanged := lastSyncedRemoteMD5 != "" && remoteFile.MD5Checksum != "" && lastSyncedRemoteMD5 != remoteFile.MD5Checksum
			localChanged := localChangedSinceSync(localFile, lastSyncedInfo)

			if !remoteChanged {
				if localChanged {
					return types.ActionUploadUpdate
				}
				return types.ActionSkipNoChange
			}

			if !localChanged {
				return types.ActionDownloadUpdate
			}
		}

		// Fallback for first sync or ambiguous metadata: retain local-protect policy.
		buffer := 2 * time.Second
		if remoteFile.ModifiedTime.After(localFile.ModTime.Add(buffer)) {
			return types.ActionDownloadUpdate
		}

		// Local is newer or we can't decide based on time, treat as conflict.
		return types.ActionUploadConflict // Local version wins in this implementation
	}

	return types.ActionSkipNoChange
}
