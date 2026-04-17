package core

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/scanner"
	"gdrive-bisync/internal/types"
)

type fakeDriveService struct {
	mu sync.Mutex

	startPageToken string
	newPageToken   string
	changes        []*drive.Change
	fullScanFiles  types.DriveFileMap

	createCalls []fakeCreateFolderCall
	uploadCalls []fakeUploadCall
	trashCalls  []string
}

type fakeCreateFolderCall struct {
	parentID string
	name     string
}

type fakeUploadCall struct {
	localPath string
	remoteInfo struct {
		Name     string
		FolderID string
		FileID   string
	}
}

func (f *fakeDriveService) GetStartPageToken() (string, error) {
	return f.startPageToken, nil
}

func (f *fakeDriveService) ListFilesRecursive(_ string, _ []*regexp.Regexp, _ func(path string), _, _ int) (types.DriveFileMap, error) {
	cloned := make(types.DriveFileMap, len(f.fullScanFiles))
	for path, file := range f.fullScanFiles {
		copyFile := *file
		cloned[path] = &copyFile
	}
	return cloned, nil
}

func (f *fakeDriveService) FetchChanges(_ string) ([]*drive.Change, string, error) {
	return f.changes, f.newPageToken, nil
}

func (f *fakeDriveService) DownloadFile(_, destinationPath string) error {
	return os.WriteFile(destinationPath, []byte("downloaded"), 0644)
}

func (f *fakeDriveService) UploadOrUpdateFile(localPath string, remoteInfo struct {
	Name     string
	FolderID string
	FileID   string
}) (*drive.File, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, os.ErrInvalid
	}

	md5Checksum, err := scanner.GetFileMD5(localPath)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadCalls = append(f.uploadCalls, fakeUploadCall{localPath: localPath, remoteInfo: remoteInfo})

	id := remoteInfo.FileID
	if id == "" {
		id = "upload-" + remoteInfo.Name
	}

	return &drive.File{
		Id:           id,
		Name:         remoteInfo.Name,
		ModifiedTime: time.Now().Format(time.RFC3339),
		Md5Checksum:  md5Checksum,
	}, nil
}

func (f *fakeDriveService) CreateFolder(parentFolderID string, folderName string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls = append(f.createCalls, fakeCreateFolderCall{parentID: parentFolderID, name: folderName})
	return "folder-" + folderName, nil
}

func (f *fakeDriveService) TrashRemoteFile(fileID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trashCalls = append(f.trashCalls, fileID)
	return nil
}

func testConfig(root string) *config.Config {
	return &config.Config{
		LocalSyncPath:          root,
		RemoteFolderID:         "root",
		DBFileName:             ".db",
		MetadataFileName:       ".meta",
		StateFileName:          ".state",
		WatchDebounceDelay:     1,
		PeriodicSyncIntervalMs: 1,
		MaxConcurrentScans:     1,
		MaxConcurrentDownloads: 1,
		MaxConcurrentUploads:   1,
		MaxRetries:             1,
		IgnoreRegexps:          []*regexp.Regexp{},
	}
}

func writeTestFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	absolutePath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", relativePath, err)
	}
}

func seedLocalMetadata(t *testing.T, root string, metadata map[string]*types.FileMetadata, relativePaths ...string) {
	t.Helper()
	for _, relativePath := range relativePaths {
		absolutePath := filepath.Join(root, relativePath)
		info, err := os.Stat(absolutePath)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", relativePath, err)
		}
		md5Checksum, err := scanner.GetFileMD5(absolutePath)
		if err != nil {
			t.Fatalf("failed to hash %s: %v", relativePath, err)
		}
		metadata[relativePath] = &types.FileMetadata{
			LocalMD5Checksum: md5Checksum,
			LocalModTime:     info.ModTime(),
			LocalSize:        info.Size(),
		}
	}
}

func assertPathExists(t *testing.T, root, relativePath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
		t.Fatalf("expected %s to exist: %v", relativePath, err)
	}
}

func assertPathMissing(t *testing.T, root, relativePath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, relativePath)); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", relativePath, err)
	}
}

func assertRemoteHasPath(t *testing.T, remoteFiles types.DriveFileMap, relativePath string, isDir bool) {
	t.Helper()
	file, ok := remoteFiles[relativePath]
	if !ok {
		t.Fatalf("expected remote path %s to exist", relativePath)
	}
	if file.IsDirectory != isDir {
		t.Fatalf("expected remote path %s directory=%v, got %v", relativePath, isDir, file.IsDirectory)
	}
}

func assertRemoteMissingPath(t *testing.T, remoteFiles types.DriveFileMap, relativePath string) {
	t.Helper()
	if _, ok := remoteFiles[relativePath]; ok {
		t.Fatalf("expected remote path %s to be absent", relativePath)
	}
}

func TestSync_LocalRestructureWithDeletedSiblingNote_ConvergesToLocalShape(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Server/IP.md", "10.0.0.1")
	writeTestFile(t, root, "Server/Admin.md", "admin")
	writeTestFile(t, root, "Server/SSH.md", "ssh")

	metadata := map[string]*types.FileMetadata{}
	seedLocalMetadata(t, root, metadata, "Server/IP.md", "Server/Admin.md", "Server/SSH.md")
	metadata["Account"] = &types.FileMetadata{}
	metadata["Account/IP.md"] = &types.FileMetadata{RemoteMD5Checksum: "old"}
	metadata["Account/Admin.md"] = &types.FileMetadata{RemoteMD5Checksum: "old"}
	metadata["Account/SSH.md"] = &types.FileMetadata{RemoteMD5Checksum: "old"}
	metadata["Server.md"] = &types.FileMetadata{RemoteMD5Checksum: "old"}

	remoteFiles := types.DriveFileMap{
		"Account":          {ID: "dir-account", Path: "Account", Name: "Account", IsDirectory: true},
		"Account/IP.md":    {ID: "old-ip", Path: "Account/IP.md", Name: "IP.md", MD5Checksum: "old"},
		"Account/Admin.md": {ID: "old-admin", Path: "Account/Admin.md", Name: "Admin.md", MD5Checksum: "old"},
		"Account/SSH.md":   {ID: "old-ssh", Path: "Account/SSH.md", Name: "SSH.md", MD5Checksum: "old"},
		"Server.md":        {ID: "old-server-note", Path: "Server.md", Name: "Server.md", MD5Checksum: "old"},
	}

	fakeDrive := &fakeDriveService{newPageToken: "next-token"}
	cfg := testConfig(root)
	pageToken := "current-token"

	if err := Sync(fakeDrive, remoteFiles, metadata, cfg, &pageToken, nil); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if pageToken != "next-token" {
		t.Fatalf("expected page token to advance, got %s", pageToken)
	}

	assertPathExists(t, root, "Server/IP.md")
	assertPathExists(t, root, "Server/Admin.md")
	assertPathExists(t, root, "Server/SSH.md")
	assertPathMissing(t, root, "Account")
	assertPathMissing(t, root, "Server.md")
	assertRemoteHasPath(t, remoteFiles, "Server", true)
	assertRemoteHasPath(t, remoteFiles, "Server/IP.md", false)
	assertRemoteHasPath(t, remoteFiles, "Server/Admin.md", false)
	assertRemoteHasPath(t, remoteFiles, "Server/SSH.md", false)
	assertRemoteMissingPath(t, remoteFiles, "Account")
	assertRemoteMissingPath(t, remoteFiles, "Account/IP.md")
	assertRemoteMissingPath(t, remoteFiles, "Account/Admin.md")
	assertRemoteMissingPath(t, remoteFiles, "Account/SSH.md")
	assertRemoteMissingPath(t, remoteFiles, "Server.md")

	if len(fakeDrive.createCalls) != 1 || fakeDrive.createCalls[0].name != "Server" {
		t.Fatalf("expected one remote folder creation for Server, got %#v", fakeDrive.createCalls)
	}
	if len(fakeDrive.uploadCalls) != 3 {
		t.Fatalf("expected 3 uploads into new Server folder, got %d", len(fakeDrive.uploadCalls))
	}
}

func TestSync_RemoteFileReplacedByLocalFolder_RecreatesFolderAndUploadsChildren(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Docs/Plan.md", "plan")

	remoteFiles := types.DriveFileMap{
		"Docs": {ID: "remote-docs-file", Path: "Docs", Name: "Docs", MD5Checksum: "old"},
	}
	metadata := map[string]*types.FileMetadata{}
	fakeDrive := &fakeDriveService{newPageToken: "token-2"}
	cfg := testConfig(root)
	pageToken := "token-1"

	if err := Sync(fakeDrive, remoteFiles, metadata, cfg, &pageToken, nil); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertPathExists(t, root, "Docs/Plan.md")
	assertRemoteHasPath(t, remoteFiles, "Docs", true)
	assertRemoteHasPath(t, remoteFiles, "Docs/Plan.md", false)

	if len(fakeDrive.trashCalls) == 0 || fakeDrive.trashCalls[0] != "remote-docs-file" {
		t.Fatalf("expected remote file at Docs to be trashed first, got %#v", fakeDrive.trashCalls)
	}
	if len(fakeDrive.createCalls) != 1 || fakeDrive.createCalls[0].name != "Docs" {
		t.Fatalf("expected Docs folder to be recreated remotely, got %#v", fakeDrive.createCalls)
	}
	if len(fakeDrive.uploadCalls) != 1 || filepath.Base(fakeDrive.uploadCalls[0].localPath) != "Plan.md" {
		t.Fatalf("expected one upload for Docs/Plan.md, got %#v", fakeDrive.uploadCalls)
	}
}

func TestSync_RemoteFolderReplacedByLocalFile_RemovesChildrenAndUploadsFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "Notes", "single note")

	remoteFiles := types.DriveFileMap{
		"Notes":        {ID: "remote-notes-folder", Path: "Notes", Name: "Notes", IsDirectory: true},
		"Notes/Old.md": {ID: "remote-old-child", Path: "Notes/Old.md", Name: "Old.md", MD5Checksum: "old"},
	}
	metadata := map[string]*types.FileMetadata{}
	fakeDrive := &fakeDriveService{newPageToken: "token-2"}
	cfg := testConfig(root)
	pageToken := "token-1"

	if err := Sync(fakeDrive, remoteFiles, metadata, cfg, &pageToken, nil); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	assertPathExists(t, root, "Notes")
	assertRemoteHasPath(t, remoteFiles, "Notes", false)
	assertRemoteMissingPath(t, remoteFiles, "Notes/Old.md")

	if len(fakeDrive.trashCalls) == 0 || fakeDrive.trashCalls[0] != "remote-notes-folder" {
		t.Fatalf("expected remote Notes folder to be trashed first, got %#v", fakeDrive.trashCalls)
	}
	if len(fakeDrive.uploadCalls) != 1 || filepath.Base(fakeDrive.uploadCalls[0].localPath) != "Notes" {
		t.Fatalf("expected one upload for local Notes file, got %#v", fakeDrive.uploadCalls)
	}
}
