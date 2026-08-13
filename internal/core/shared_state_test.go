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

func TestRunMutationSerializesCallers(t *testing.T) {
	state := NewSharedState(types.DriveFileMap{}, map[string]*types.FileMetadata{}, "")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go state.RunMutation(func() { close(entered); <-release })
	<-entered
	go func() { state.RunMutation(func() {}); close(done) }()
	select {
	case <-done:
		t.Fatal("second mutation entered concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second mutation never ran")
	}
}
