package core

import (
	"path/filepath"

	"gdrive-bisync/internal/types"
)

func resolveRemoteParentFolderID(path string, remoteFiles types.DriveFileMap, rootFolderID string) (string, bool) {
	parentPath := filepath.Dir(path)
	if parentPath == "." {
		return rootFolderID, true
	}

	parent, ok := remoteFiles[parentPath]
	if !ok || !parent.IsDirectory {
		return "", false
	}

	return parent.ID, true
}

func resolveParentFolderIDFromSharedState(path string, sharedState *SharedState, rootFolderID string) (string, bool) {
	parentFolderID := ""
	parentExists := false

	sharedState.ReadRemoteFiles(func(remoteFiles types.DriveFileMap, _ map[string]*types.FileMetadata) {
		parentFolderID, parentExists = resolveRemoteParentFolderID(path, remoteFiles, rootFolderID)
	})

	return parentFolderID, parentExists
}

func upsertRemoteMetadata(metadata map[string]*types.FileMetadata, path string, remoteMD5 string) {
	if existing, exists := metadata[path]; exists {
		existing.RemoteMD5Checksum = remoteMD5
		return
	}

	metadata[path] = &types.FileMetadata{RemoteMD5Checksum: remoteMD5}
}
