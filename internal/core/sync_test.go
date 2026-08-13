package core

import (
	"testing"
	"time"

	"gdrive-bisync/internal/types"
)

func TestDetermineTaskAction_WhenFileIsUnderNewRemoteFolder_UploadsNew(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"Server/IP.md": {
			RemoteMD5Checksum: "same-md5",
			LocalMD5Checksum:  "same-md5",
			LocalModTime:      now,
		},
	}

	action := determineTaskAction(
		"Server/IP.md",
		&types.LocalFile{Path: "Server/IP.md", MD5Checksum: "same-md5", ModTime: now},
		nil,
		metadata,
		map[string]struct{}{"Server": {}},
	)

	if action != types.ActionUploadNew {
		t.Fatalf("expected %v, got %v", types.ActionUploadNew, action)
	}
}

func TestDetermineTaskAction_WhenFileIsNotUnderNewRemoteFolder_KeepsDeleteLocal(t *testing.T) {
	now := time.Now()
	metadata := map[string]*types.FileMetadata{
		"Server/IP.md": {
			RemoteMD5Checksum: "same-md5",
			LocalMD5Checksum:  "same-md5",
			LocalModTime:      now,
		},
	}

	action := determineTaskAction(
		"Server/IP.md",
		&types.LocalFile{Path: "Server/IP.md", MD5Checksum: "same-md5", ModTime: now},
		nil,
		metadata,
		map[string]struct{}{},
	)

	if action != types.ActionDeleteLocal {
		t.Fatalf("expected %v, got %v", types.ActionDeleteLocal, action)
	}
}

func TestRemoveRemotePathAndMetadata_WhenPathIsDirectory_RemovesChildren(t *testing.T) {
	remoteFiles := types.DriveFileMap{
		"Server":       {Path: "Server", IsDirectory: true},
		"Server/IP.md": {Path: "Server/IP.md"},
		"Other.md":     {Path: "Other.md"},
	}
	metadata := map[string]*types.FileMetadata{
		"Server":       {},
		"Server/IP.md": {},
		"Other.md":     {},
	}

	deleted := removeRemotePathAndMetadata(remoteFiles, metadata, "Server")

	if _, ok := remoteFiles["Server"]; ok {
		t.Fatal("expected Server to be removed from remote files")
	}
	if _, ok := remoteFiles["Server/IP.md"]; ok {
		t.Fatal("expected Server/IP.md to be removed from remote files")
	}
	if _, ok := remoteFiles["Other.md"]; !ok {
		t.Fatal("expected unrelated remote file to remain")
	}

	if _, ok := metadata["Server"]; ok {
		t.Fatal("expected Server metadata to be removed")
	}
	if _, ok := metadata["Server/IP.md"]; ok {
		t.Fatal("expected Server/IP.md metadata to be removed")
	}
	if _, ok := metadata["Other.md"]; !ok {
		t.Fatal("expected unrelated metadata to remain")
	}

	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted metadata paths, got %d", len(deleted))
	}
}

func TestShouldDeleteRemoteDirectory_WhenFolderMetadataExists_ReturnsTrue(t *testing.T) {
	if !shouldDeleteRemoteDirectory("Account", types.LocalFileMap{}, map[string]*types.FileMetadata{"Account": {}}) {
		t.Fatal("expected synced remote directory to be deleted when local folder is gone")
	}
}

func TestShouldDeleteRemoteDirectory_WhenOnlyChildMetadataExists_ReturnsTrue(t *testing.T) {
	if !shouldDeleteRemoteDirectory("Account", types.LocalFileMap{}, map[string]*types.FileMetadata{"Account/IP.md": {}}) {
		t.Fatal("expected stale remote directory to be deleted when only synced children remain")
	}
}

func TestShouldDeleteRemoteDirectory_WhenLocalChildrenExist_ReturnsFalse(t *testing.T) {
	localFiles := types.LocalFileMap{
		"Account/IP.md": {Path: "Account/IP.md"},
	}
	if shouldDeleteRemoteDirectory("Account", localFiles, map[string]*types.FileMetadata{"Account/IP.md": {}}) {
		t.Fatal("expected remote directory to remain while local descendants still exist")
	}
}
