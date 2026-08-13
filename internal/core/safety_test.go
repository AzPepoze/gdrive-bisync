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
	cfg := &config.Config{MaxDeletionsPerSync: 1}
	if err := validateDeletionSafety(tasks, 100, 100, cfg, false); err == nil {
		t.Fatal("expected deletion count threshold to reject plan")
	}
}

func TestValidateDeletionSafetyAllowsExplicitOverride(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteLocal, FilePath: "one"}}
	cfg := &config.Config{MaxDeletionsPerSync: 0, MaxDeletionPercent: 0.1}
	if err := validateDeletionSafety(tasks, 1, 1, cfg, true); err != nil {
		t.Fatalf("explicit override should allow plan: %v", err)
	}
}

func TestValidateDeletionSafetyRejectsRemoteDeletionAgainstEmptyBaseline(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteRemote, FilePath: "one"}}
	cfg := &config.Config{MaxDeletionPercent: 50}
	if err := validateDeletionSafety(tasks, 0, 0, cfg, false); err == nil {
		t.Fatal("expected empty remote baseline to reject deletion")
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
