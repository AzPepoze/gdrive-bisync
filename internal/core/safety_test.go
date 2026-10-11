package core

import (
	"testing"

	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/types"
)

func TestValidateDeletionSafetyRejectsCountThreshold(t *testing.T) {
	tasks := []types.SyncTask{
		{Action: types.ActionDeleteLocal, FilePath: "one"},
		{Action: types.ActionDeleteRemote, FilePath: "two"},
	}
	localFiles := types.LocalFileMap{"one": {Path: "one", MD5Checksum: "a"}}
	remoteFiles := types.DriveFileMap{"two": {Path: "two", MD5Checksum: "b"}}
	cfg := &config.Config{MaxDeletionsPerSync: 1}
	if err := validateDeletionSafety(tasks, localFiles, remoteFiles, cfg, false); err == nil {
		t.Fatal("expected deletion count threshold to reject plan")
	}
}

func TestValidateDeletionSafetyAllowsExplicitOverride(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteLocal, FilePath: "one"}}
	localFiles := types.LocalFileMap{"one": {Path: "one", MD5Checksum: "a"}}
	cfg := &config.Config{MaxDeletionsPerSync: 0, MaxDeletionPercent: 0.1}
	if err := validateDeletionSafety(tasks, localFiles, types.DriveFileMap{}, cfg, true); err != nil {
		t.Fatalf("explicit override should allow plan: %v", err)
	}
}

func TestValidateDeletionSafetyRejectsRemoteDeletionAgainstEmptyBaseline(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "one"}}
	remoteFiles := types.DriveFileMap{"one": {Path: "one", MD5Checksum: "a"}}
	cfg := &config.Config{MaxDeletionPercent: 50}
	if err := validateDeletionSafety(tasks, types.LocalFileMap{}, remoteFiles, cfg, false); err == nil {
		t.Fatal("expected empty remote baseline to reject deletion")
	}
}

func TestValidateDeletionSafetyIgnoresMovedFiles(t *testing.T) {
	tasks := []types.SyncTask{
		{Action: types.ActionDeleteRemote, FilePath: "old/a.txt"},
		{Action: types.ActionDeleteRemote, FilePath: "old/b.txt"},
		{Action: types.ActionUploadNew, FilePath: "new/a.txt"},
		{Action: types.ActionUploadNew, FilePath: "new/b.txt"},
	}
	localFiles := types.LocalFileMap{
		"new/a.txt": {Path: "new/a.txt", MD5Checksum: "a"},
		"new/b.txt": {Path: "new/b.txt", MD5Checksum: "b"},
	}
	remoteFiles := types.DriveFileMap{
		"old/a.txt": {Path: "old/a.txt", MD5Checksum: "a"},
		"old/b.txt": {Path: "old/b.txt", MD5Checksum: "b"},
	}
	cfg := &config.Config{MaxDeletionsPerSync: 1}
	if err := validateDeletionSafety(tasks, localFiles, remoteFiles, cfg, false); err != nil {
		t.Fatalf("moved files should not count as deletions: %v", err)
	}
}

func TestValidateDeletionSafetyCountsDeleteWithoutCopy(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "old/a.txt"}}
	remoteFiles := types.DriveFileMap{"old/a.txt": {Path: "old/a.txt", MD5Checksum: "a"}}
	localFiles := types.LocalFileMap{"other.txt": {Path: "other.txt", MD5Checksum: "b"}}
	cfg := &config.Config{MaxDeletionsPerSync: 0, MaxDeletionPercent: 50}
	if err := validateDeletionSafety(tasks, localFiles, remoteFiles, cfg, false); err == nil {
		t.Fatal("delete with no copy elsewhere must still count")
	}
}

func TestValidateDeletionSafetyDoesNotPairWithFileBeingDeleted(t *testing.T) {
	tasks := []types.SyncTask{
		{Action: types.ActionDeleteRemote, FilePath: "a.txt"},
		{Action: types.ActionDeleteLocal, FilePath: "b.txt"},
	}
	localFiles := types.LocalFileMap{"b.txt": {Path: "b.txt", MD5Checksum: "x"}}
	remoteFiles := types.DriveFileMap{"a.txt": {Path: "a.txt", MD5Checksum: "x"}}
	cfg := &config.Config{MaxDeletionsPerSync: 1}
	if err := validateDeletionSafety(tasks, localFiles, remoteFiles, cfg, false); err == nil {
		t.Fatal("content only copied to a file that is also deleted must still count")
	}
}

func TestValidateDeletionSafetyDoesNotPairEmptyChecksum(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "a.txt"}}
	localFiles := types.LocalFileMap{"b.txt": {Path: "b.txt"}}
	remoteFiles := types.DriveFileMap{"a.txt": {Path: "a.txt"}}
	cfg := &config.Config{MaxDeletionsPerSync: 0, MaxDeletionPercent: 50}
	if err := validateDeletionSafety(tasks, localFiles, remoteFiles, cfg, false); err == nil {
		t.Fatal("files without checksums must not be paired")
	}
}

func TestCollapseLocalDeletionSubtreesKeepsParentAsSingleRestoreUnit(t *testing.T) {
	tasks := []types.SyncTask{
		{Action: types.ActionDeleteLocal, FilePath: "folder/file.txt"},
		{Action: types.ActionDeleteLocal, FilePath: "folder"},
		{Action: types.ActionUploadNew, FilePath: "other.txt"},
	}
	localFiles := types.LocalFileMap{
		"folder":          {Path: "folder", IsDirectory: true},
		"folder/file.txt": {Path: "folder/file.txt"},
	}
	collapsed := collapseLocalDeletionSubtrees(tasks, localFiles)
	if len(collapsed) != 2 || collapsed[0].FilePath != "folder" || collapsed[1].FilePath != "other.txt" {
		t.Fatalf("unexpected collapsed tasks: %#v", collapsed)
	}
}

func TestCollapseDeletionSubtreesKeepsRemoteParentOnly(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "folder/file.txt"}, {Action: types.ActionDeleteRemote, FilePath: "folder"}}
	remote := types.DriveFileMap{"folder": {Path: "folder", IsDirectory: true}, "folder/file.txt": {Path: "folder/file.txt"}}
	collapsed := collapseDeletionSubtrees(tasks, types.LocalFileMap{}, remote)
	if len(collapsed) != 1 || collapsed[0].FilePath != "folder" {
		t.Fatalf("unexpected collapsed tasks: %#v", collapsed)
	}
}

func TestRemoteDescendantUsesDriveSeparatorsOnEveryPlatform(t *testing.T) {
	if !isRemoteDescendant("folder/file.txt", "folder") {
		t.Fatal("slash-normalized remote child was not recognized")
	}
	if !isRemoteDescendant(`folder\file.txt`, "folder") {
		t.Fatal("backslash remote child was not normalized")
	}
	if isRemoteDescendant("folder-two/file.txt", "folder") {
		t.Fatal("prefix sibling was mistaken for descendant")
	}
}

func TestSyncRejectsNilPageToken(t *testing.T) {
	if err := Sync(nil, nil, nil, &config.Config{}, nil, nil, nil, SyncOptions{}); err == nil {
		t.Fatal("expected nil page token to be rejected")
	}
}
