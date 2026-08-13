package core

import (
	"testing"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/types"
)

func TestApplyChangesRemoteDeletionRetainsMetadataForReconciliation(t *testing.T) {
	remoteFiles := types.DriveFileMap{
		"notes.txt": {ID: "remote-notes", Path: "notes.txt"},
	}
	metadata := map[string]*types.FileMetadata{
		"notes.txt": {RemoteMD5Checksum: "previous-remote-md5", LocalMD5Checksum: "local-md5"},
	}

	applyChanges([]*drive.Change{{FileId: "remote-notes", Removed: true}}, remoteFiles, metadata, "root", nil)

	if remoteFiles["notes.txt"] != nil {
		t.Fatal("expected remote record to be removed")
	}
	if metadata["notes.txt"] == nil {
		t.Fatal("expected metadata proof to remain until local deletion succeeds")
	}
}
