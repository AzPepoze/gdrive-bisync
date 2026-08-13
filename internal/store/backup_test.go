package store

import (
	"os"
	"path/filepath"
	"testing"

	"gdrive-bisync/internal/types"
)

func TestBackupDatabaseCopiesAndRotates(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "state.db")
	for version := byte('1'); version <= '4'; version++ {
		if err := os.WriteFile(databasePath, []byte{version}, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupDatabase(databasePath, 2); err != nil {
			t.Fatalf("backup version %c: %v", version, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(directory, ".gdrive-bisync-backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(entries))
	}
}

func TestReplaceSyncStatePersistsOneConsistentSnapshot(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	database, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	remote := types.DriveFileMap{"a.txt": {ID: "id-a", Path: "a.txt"}}
	metadata := map[string]*types.FileMetadata{"a.txt": {RemoteMD5Checksum: "hash"}}
	if err := database.ReplaceSyncState(remote, metadata, "token-a"); err != nil {
		t.Fatal(err)
	}
	loadedRemote, err := database.LoadRemoteFiles()
	if err != nil {
		t.Fatal(err)
	}
	loadedMetadata, err := database.LoadMetadata()
	if err != nil {
		t.Fatal(err)
	}
	loadedToken, err := database.LoadPageToken()
	if err != nil {
		t.Fatal(err)
	}
	if loadedRemote["a.txt"].ID != "id-a" || loadedMetadata["a.txt"].RemoteMD5Checksum != "hash" || loadedToken != "token-a" {
		t.Fatal("sync snapshot was not persisted consistently")
	}
}
