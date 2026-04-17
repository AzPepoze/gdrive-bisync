package core

import (
	"testing"
	"time"

	"gdrive-bisync/internal/types"
)

func TestDetermineSyncAction_WhenLocalMissingAndRemoteUnchanged_DeletesRemote(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "remote-md5",
			LocalMD5Checksum:  "local-md5",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		nil,
		&types.DriveFile{Path: "a.txt", MD5Checksum: "remote-md5", ModifiedTime: now},
		metadata,
	)

	if action != types.ActionDeleteRemote {
		t.Fatalf("expected %v, got %v", types.ActionDeleteRemote, action)
	}
}

func TestDetermineSyncAction_WhenLocalMissingAndRemoteChanged_DownloadsRemote(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "old-remote-md5",
			LocalMD5Checksum:  "local-md5",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		nil,
		&types.DriveFile{Path: "a.txt", MD5Checksum: "new-remote-md5", ModifiedTime: now.Add(time.Second)},
		metadata,
	)

	if action != types.ActionDownloadUpdate {
		t.Fatalf("expected %v, got %v", types.ActionDownloadUpdate, action)
	}
}

func TestDetermineSyncAction_WhenRemoteMissingAndLocalChanged_UploadsNew(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "remote-md5",
			LocalMD5Checksum:  "old-local-md5",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		&types.LocalFile{Path: "a.txt", MD5Checksum: "new-local-md5", ModTime: now.Add(time.Second)},
		nil,
		metadata,
	)

	if action != types.ActionUploadNew {
		t.Fatalf("expected %v, got %v", types.ActionUploadNew, action)
	}
}

func TestDetermineSyncAction_WhenBothChanged_PreservesLocalWinsPolicy(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "old-remote-md5",
			LocalMD5Checksum:  "old-local-md5",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		&types.LocalFile{Path: "a.txt", MD5Checksum: "new-local-md5", ModTime: now.Add(500 * time.Millisecond)},
		&types.DriveFile{Path: "a.txt", MD5Checksum: "new-remote-md5", ModifiedTime: now.Add(250 * time.Millisecond)},
		metadata,
	)

	if action != types.ActionUploadConflict {
		t.Fatalf("expected %v, got %v", types.ActionUploadConflict, action)
	}
}

func TestDetermineSyncAction_WhenRemoteMissingAndLocalEmptyFileRecentlyTouched_SkipsDelete(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "remote-md5",
			LocalMD5Checksum:  "d41d8cd98f00b204e9800998ecf8427e",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		&types.LocalFile{Path: "a.txt", MD5Checksum: "d41d8cd98f00b204e9800998ecf8427e", ModTime: now.Add(time.Second)},
		nil,
		metadata,
	)

	if action != types.ActionSkipNoChange {
		t.Fatalf("expected %v, got %v", types.ActionSkipNoChange, action)
	}
}

func TestDetermineSyncAction_WhenRemoteMissingAndLocalEmptyFileUnchanged_DeletesLocal(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"a.txt": {
			RemoteMD5Checksum: "remote-md5",
			LocalMD5Checksum:  "d41d8cd98f00b204e9800998ecf8427e",
			LocalModTime:      now,
		},
	}

	action := DetermineSyncAction(
		"a.txt",
		&types.LocalFile{Path: "a.txt", MD5Checksum: "d41d8cd98f00b204e9800998ecf8427e", ModTime: now},
		nil,
		metadata,
	)

	if action != types.ActionDeleteLocal {
		t.Fatalf("expected %v, got %v", types.ActionDeleteLocal, action)
	}
}
