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
	if err := validateDeletionSafety(tasks, 100, cfg, false); err == nil {
		t.Fatal("expected deletion count threshold to reject plan")
	}
}

func TestValidateDeletionSafetyAllowsExplicitOverride(t *testing.T) {
	tasks := []types.SyncTask{{Action: types.ActionDeleteLocal, FilePath: "one"}}
	cfg := &config.Config{MaxDeletionsPerSync: 0, MaxDeletionPercent: 0.1}
	if err := validateDeletionSafety(tasks, 1, cfg, true); err != nil {
		t.Fatalf("explicit override should allow plan: %v", err)
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
