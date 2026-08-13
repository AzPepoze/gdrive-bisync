package core

import (
	"testing"
	"time"

	"gdrive-bisync/internal/types"
)

func TestSharedState_TracksDownloadsDuringExclusiveSync(t *testing.T) {
	sharedState := NewSharedState(types.DriveFileMap{}, map[string]*types.FileMetadata{}, "")
	const downloadPath = "fixtures/downloaded-file"
	finished := make(chan struct{})

	go func() {
		sharedState.RunExclusive(func(types.DriveFileMap, map[string]*types.FileMetadata) {
			sharedState.AddActiveDownload(downloadPath)
			defer sharedState.RemoveActiveDownload(downloadPath)
			if !sharedState.IsActiveDownload(downloadPath) {
				t.Error("expected active download to be observable during sync")
			}
		})
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("tracking a download while syncing deadlocked")
	}
}
