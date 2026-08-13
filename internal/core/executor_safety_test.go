package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/types"
)

func TestExecutorReturnsRemoteDeletionFailure(t *testing.T) {
	driveError := errors.New("drive unavailable")
	drive := &fakeDriveService{trashErr: driveError}
	remoteFiles := types.DriveFileMap{"lost.txt": {ID: "remote-id", Path: "lost.txt"}}
	executor := NewExecutor(drive, remoteFiles, map[string]*types.FileMetadata{"lost.txt": {}}, &config.Config{MaxConcurrentDownloads: 1, MaxConcurrentUploads: 1}, t.TempDir(), nil, false)
	err := executor.ExecuteTasks([]types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "lost.txt"}})
	if !errors.Is(err, driveError) {
		t.Fatalf("expected deletion error, got %v", err)
	}
	if remoteFiles["lost.txt"] == nil {
		t.Fatal("failed deletion must retain remote state")
	}
}

func TestExecutorRejectsUploadWhenRemoteParentIsMissing(t *testing.T) {
	executor := NewExecutor(&fakeDriveService{}, types.DriveFileMap{}, map[string]*types.FileMetadata{}, &config.Config{RemoteFolderID: "root", MaxConcurrentDownloads: 1, MaxConcurrentUploads: 1}, t.TempDir(), nil, false)
	err := executor.ExecuteTasks([]types.SyncTask{{Action: types.ActionUploadNew, FilePath: "missing/file.txt"}})
	if err == nil {
		t.Fatal("expected missing remote parent to fail the task")
	}
}

func TestLocalPathWithinRootRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../secret", "/tmp/secret", "."} {
		if _, err := localPathWithinRoot(root, path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestWaitForRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if !errors.Is(waitForRetry(ctx, time.Hour), context.Canceled) {
		t.Fatal("expected cancellation")
	}
	if time.Since(started) > time.Second {
		t.Fatal("canceled retry wait did not return promptly")
	}
}

func TestRemoteFolderDeletionRemovesSlashAndBackslashDescendants(t *testing.T) {
	drive := &fakeDriveService{}
	remoteFiles := types.DriveFileMap{
		"folder":               {ID: "folder-id", Path: "folder", IsDirectory: true},
		"folder/slash.txt":     {ID: "slash-id", Path: "folder/slash.txt"},
		`folder\backslash.txt`: {ID: "backslash-id", Path: `folder\backslash.txt`},
	}
	metadata := map[string]*types.FileMetadata{"folder": {}, "folder/slash.txt": {}, `folder\backslash.txt`: {}}
	executor := NewExecutor(drive, remoteFiles, metadata, &config.Config{MaxConcurrentDownloads: 1, MaxConcurrentUploads: 1}, t.TempDir(), nil, false)
	if err := executor.ExecuteTasks([]types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "folder"}}); err != nil {
		t.Fatal(err)
	}
	if len(remoteFiles) != 0 || len(metadata) != 0 {
		t.Fatalf("folder descendants remain: remote=%v metadata=%v", remoteFiles, metadata)
	}
}
