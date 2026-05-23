package core

import (
	"sync"
	"time"

	"gdrive-bisync/internal/types"
)

type SharedState struct {
	mutex              sync.RWMutex
	remoteFiles        types.DriveFileMap
	metadata           map[string]*types.FileMetadata
	pageToken          string
	activeDownloads    map[string]time.Time
	completedDownloads map[string]time.Time
}

func NewSharedState(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata, pageToken string) *SharedState {
	return &SharedState{
		remoteFiles:        remoteFiles,
		metadata:           metadata,
		pageToken:          pageToken,
		activeDownloads:    make(map[string]time.Time),
		completedDownloads: make(map[string]time.Time),
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

func (sharedState *SharedState) AddActiveDownload(path string) {
	sharedState.mutex.Lock()
	defer sharedState.mutex.Unlock()
	sharedState.activeDownloads[path] = time.Now()
}

func (sharedState *SharedState) RemoveActiveDownload(path string) {
	sharedState.mutex.Lock()
	defer sharedState.mutex.Unlock()
	delete(sharedState.activeDownloads, path)
	sharedState.completedDownloads[path] = time.Now()
}

func (sharedState *SharedState) IsActiveDownload(path string) bool {
	sharedState.mutex.RLock()
	defer sharedState.mutex.RUnlock()
	if _, active := sharedState.activeDownloads[path]; active {
		return true
	}
	if t, completed := sharedState.completedDownloads[path]; completed {
		return time.Since(t) < 5*time.Second
	}
	return false
}
