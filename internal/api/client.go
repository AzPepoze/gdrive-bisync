package api

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"gdrive-bisync/internal/types"
)

type DriveClient interface {
	GetStartPageToken(ctx context.Context) (string, error)
	ListFilesRecursive(ctx context.Context, request ListFilesRequest) (types.DriveFileMap, error)
	FetchChanges(ctx context.Context, request FetchChangesRequest) (*FetchChangesResult, error)
	DownloadFile(ctx context.Context, request DownloadFileRequest) error
	UploadOrUpdateFile(ctx context.Context, request UploadFileRequest) (*types.DriveFile, error)
	CreateFolder(ctx context.Context, request CreateFolderRequest) (*types.DriveFile, error)
	TrashRemoteFile(ctx context.Context, fileID string) error
}

type ListFilesRequest struct {
	FolderID      string
	IgnoreRegexps []*regexp.Regexp
	OnProgress    func(path string)
	MaxConcurrent int
	MaxRetries    int
}

type FetchChangesRequest struct {
	PageToken string
	PageSize  int64
}

type FetchChangesResult struct {
	Changes      []*drive.Change
	NewPageToken string
}

type DownloadFileRequest struct {
	FileID          string
	DestinationPath string
}

type UploadFileRequest struct {
	LocalPath  string
	RemotePath string
	Name       string
	FolderID   string
	FileID     string
}

type CreateFolderRequest struct {
	ParentFolderID string
	FolderName     string
	RemotePath     string
}

type DriveService struct {
	srv *drive.Service
}

var _ DriveClient = (*DriveService)(nil)

func NewDriveService(client *http.Client) (*DriveService, error) {
	return newDriveService(client)
}

func newDriveService(client *http.Client, opts ...option.ClientOption) (*DriveService, error) {
	options := make([]option.ClientOption, 0, len(opts)+1)
	options = append(options, option.WithHTTPClient(client))
	options = append(options, opts...)

	srv, err := drive.NewService(context.Background(), options...)
	if err != nil {
		return nil, err
	}

	return &DriveService{srv: srv}, nil
}

type retryConfig struct {
	maxRetries int
}

func normalizeRetryConfig(maxRetries int) retryConfig {
	if maxRetries <= 0 {
		maxRetries = 10
	}
	return retryConfig{maxRetries: maxRetries}
}

func normalizeListRequest(request ListFilesRequest) ListFilesRequest {
	if request.MaxConcurrent <= 0 {
		request.MaxConcurrent = 20
	}
	if request.MaxRetries <= 0 {
		request.MaxRetries = 10
	}
	return request
}

func normalizeFetchChangesRequest(request FetchChangesRequest) FetchChangesRequest {
	if request.PageSize <= 0 {
		request.PageSize = 1000
	}
	return request
}

func parseDriveModifiedTime(raw string) time.Time {
	modifiedTime, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Now()
	}
	return modifiedTime
}
