package core

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/fsnotify/fsnotify"

	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/types"
)

func TestHandleWatchEvent_WhenPathIsDirectory_DoesNotUpload(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Server"), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	fakeDrive := &fakeDriveService{}
	sharedState := NewSharedState(types.DriveFileMap{}, map[string]*types.FileMetadata{}, "")
	cfg := &config.Config{RemoteFolderID: "root", IgnoreRegexps: []*regexp.Regexp{}}

	handleWatchEvent(
		fsnotify.Event{Name: filepath.Join(root, "Server"), Op: fsnotify.Create},
		root,
		"Server",
		fakeDrive,
		sharedState,
		cfg,
		nil,
	)

	if len(fakeDrive.uploadCalls) != 0 {
		t.Fatalf("expected no uploads for directory event, got %#v", fakeDrive.uploadCalls)
	}
}

func TestHandleWatchEvent_WhenParentFolderIsMissing_RemainsDeferred(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Server/IP.md", "10.0.0.1")

	fakeDrive := &fakeDriveService{}
	sharedState := NewSharedState(types.DriveFileMap{}, map[string]*types.FileMetadata{}, "")
	cfg := &config.Config{RemoteFolderID: "root", IgnoreRegexps: []*regexp.Regexp{}}

	handleWatchEvent(
		fsnotify.Event{Name: filepath.Join(root, "Server", "IP.md"), Op: fsnotify.Create},
		root,
		"Server/IP.md",
		fakeDrive,
		sharedState,
		cfg,
		nil,
	)

	if len(fakeDrive.uploadCalls) != 0 {
		t.Fatalf("expected no upload when remote parent folder is missing, got %#v", fakeDrive.uploadCalls)
	}
}

func TestHandleWatchEvent_WhenParentFolderExists_UploadsFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Server/IP.md", "10.0.0.1")

	fakeDrive := &fakeDriveService{}
	sharedState := NewSharedState(types.DriveFileMap{
		"Server": {ID: "remote-server-folder", Path: "Server", Name: "Server", IsDirectory: true},
	}, map[string]*types.FileMetadata{}, "")
	cfg := &config.Config{RemoteFolderID: "root", IgnoreRegexps: []*regexp.Regexp{}}

	handleWatchEvent(
		fsnotify.Event{Name: filepath.Join(root, "Server", "IP.md"), Op: fsnotify.Create},
		root,
		"Server/IP.md",
		fakeDrive,
		sharedState,
		cfg,
		nil,
	)

	if len(fakeDrive.uploadCalls) != 1 {
		t.Fatalf("expected one upload when remote parent exists, got %#v", fakeDrive.uploadCalls)
	}

	remoteSnapshot := sharedState.SnapshotRemoteFiles()
	assertRemoteHasPath(t, remoteSnapshot, "Server/IP.md", false)
}
