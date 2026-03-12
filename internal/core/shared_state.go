package core

import (
	"sync"

	"gdrive-bisync/internal/types"
)

type SharedState struct {
	mutex       sync.RWMutex
	remoteFiles types.DriveFileMap
	metadata    map[string]*types.FileMetadata
	pageToken   string
}

func NewSharedState(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata, pageToken string) *SharedState {
	return &SharedState{
		remoteFiles: remoteFiles,
		metadata:    metadata,
		pageToken:   pageToken,
	}
}

func (sharedState *SharedState) ReadRemoteFiles(callback func(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata)) {
	sharedState.mutex.RLock()
	defer sharedState.mutex.RUnlock()
	callback(sharedState.remoteFiles, sharedState.metadata)
}

func (sharedState *SharedState) WriteRemoteFiles(callback func(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata) string) {
	sharedState.mutex.Lock()
	defer sharedState.mutex.Unlock()
	token := callback(sharedState.remoteFiles, sharedState.metadata)
	if token != "" {
		sharedState.pageToken = token
	}
}

func (sharedState *SharedState) GetPageToken() string {
	sharedState.mutex.RLock()
	defer sharedState.mutex.RUnlock()
	return sharedState.pageToken
}

func (sharedState *SharedState) SetPageToken(token string) {
	sharedState.mutex.Lock()
	defer sharedState.mutex.Unlock()
	sharedState.pageToken = token
}

func (sharedState *SharedState) RunExclusive(callback func(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata)) {
	sharedState.mutex.Lock()
	defer sharedState.mutex.Unlock()
	callback(sharedState.remoteFiles, sharedState.metadata)
}

func (sharedState *SharedState) SnapshotRemoteFiles() types.DriveFileMap {
	sharedState.mutex.RLock()
	defer sharedState.mutex.RUnlock()
	snapshot := make(types.DriveFileMap, len(sharedState.remoteFiles))
	for path, driveFile := range sharedState.remoteFiles {
		snapshot[path] = driveFile
	}
	return snapshot
}
