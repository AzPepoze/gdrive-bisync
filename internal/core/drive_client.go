package core

import (
	"regexp"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/types"
)

type driveClient interface {
	GetStartPageToken() (string, error)
	ListFilesRecursive(folderID string, ignoreRegexps []*regexp.Regexp, onProgress func(path string), maxConcurrent int, maxRetries int) (types.DriveFileMap, error)
	FetchChanges(pageToken string) ([]*drive.Change, string, error)
	DownloadFile(fileID string, destinationPath string) error
	UploadOrUpdateFile(localPath string, remoteInfo struct {
		Name     string
		FolderID string
		FileID   string
	}) (*drive.File, error)
	CreateFolder(parentFolderID string, folderName string) (string, error)
	TrashRemoteFile(fileID string) error
}
