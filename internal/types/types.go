package types

import "time"

type DriveFile struct {
	ID           string
	Name         string
	Path         string
	ModifiedTime time.Time
	MD5Checksum  string
	IsDirectory  bool
}

type DriveFileMap map[string]*DriveFile

type LocalFile struct {
	Path        string
	ModTime     time.Time
	MD5Checksum string
	IsDirectory bool
}

type LocalFileMap map[string]*LocalFile

type FileMetadata struct {
	RemoteMD5Checksum string `json:"remoteMd5Checksum"`
}

type SyncAction int

const (
	ActionDownloadNew SyncAction = iota
	ActionDownloadUpdate
	ActionUploadNew
	ActionUploadUpdate
	ActionUploadConflict
	ActionDeleteLocal
	ActionDeleteRemote
	ActionSkipIdentical
	ActionSkipNoChange
)

func (s SyncAction) String() string {
	return [...]string{
		"DOWNLOAD_NEW",
		"DOWNLOAD_UPDATE",
		"UPLOAD_NEW",
		"UPLOAD_UPDATE",
		"UPLOAD_CONFLICT",
		"DELETE_LOCAL",
		"DELETE_REMOTE",
		"SKIP_IDENTICAL",
		"SKIP_NO_CHANGE",
	}[s]
}

type SyncTask struct {
	Action   SyncAction
	FilePath string
}
