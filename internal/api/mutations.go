package api

import (
	"context"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/types"
)

func (s *DriveService) CreateFolder(ctx context.Context, request CreateFolderRequest) (*types.DriveFile, error) {
	var response *drive.File
	err := s.retry(ctx, "create folder "+request.RemotePath, normalizeRetryConfig(3), func() error {
		call := s.srv.Files.Create(&drive.File{
			Name:     request.FolderName,
			MimeType: "application/vnd.google-apps.folder",
			Parents:  []string{request.ParentFolderID},
		}).
			SupportsAllDrives(true).
			Fields("id", "name", "modifiedTime").
			Context(ctx)

		var callErr error
		response, callErr = call.Do()
		return callErr
	})
	if err != nil {
		return nil, err
	}

	return &types.DriveFile{
		ID:           response.Id,
		Name:         response.Name,
		Path:         request.RemotePath,
		ModifiedTime: parseDriveModifiedTime(response.ModifiedTime),
		IsDirectory:  true,
	}, nil
}

func (s *DriveService) TrashRemoteFile(ctx context.Context, fileID string) error {
	return s.retry(ctx, "trash remote file "+fileID, normalizeRetryConfig(3), func() error {
		call := s.srv.Files.Update(fileID, &drive.File{Trashed: true}).
			SupportsAllDrives(true).
			Context(ctx)
		_, err := call.Do()
		return err
	})
}
