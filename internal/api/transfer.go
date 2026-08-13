package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

func (s *DriveService) DownloadFile(ctx context.Context, request DownloadFileRequest) error {
	logger.Debug("Downloading file from Google Drive", "fileId", request.FileID, "destination", request.DestinationPath)

	var response *drive.FilesGetCall
	response = s.srv.Files.Get(request.FileID).SupportsAllDrives(true).Context(ctx)

	httpResponse, err := response.Download()
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	temporary, err := os.CreateTemp(filepath.Dir(request.DestinationPath), ".gdrive-download-*.partial")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	written, err := io.Copy(temporary, httpResponse.Body)
	if err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, request.DestinationPath); err != nil {
		return err
	}
	logger.Debug("File downloaded successfully", "size", fmt.Sprintf("%.2f KB", float64(written)/1024), "destination", request.DestinationPath)
	return nil
}

func (s *DriveService) UploadOrUpdateFile(ctx context.Context, request UploadFileRequest) (*types.DriveFile, error) {
	fileHandle, err := os.Open(request.LocalPath)
	if err != nil {
		return nil, err
	}
	defer fileHandle.Close()

	fileInfo, err := fileHandle.Stat()
	if err == nil {
		fileSize := float64(fileInfo.Size()) / 1024
		if request.FileID != "" {
			logger.Debug("Updating file on Google Drive", "name", request.Name, "size", fmt.Sprintf("%.2f KB", fileSize), "fileId", request.FileID)
		} else {
			logger.Debug("Uploading new file to Google Drive", "name", request.Name, "size", fmt.Sprintf("%.2f KB", fileSize))
		}
	}

	var response *drive.File
	err = s.retry(ctx, fmt.Sprintf("upload %s", request.RemotePath), normalizeRetryConfig(3), func() error {
		if _, seekErr := fileHandle.Seek(0, io.SeekStart); seekErr != nil {
			return seekErr
		}

		var callErr error
		if request.FileID != "" {
			call := s.srv.Files.Update(request.FileID, &drive.File{}).
				SupportsAllDrives(true).
				Media(fileHandle).
				Fields("id", "name", "modifiedTime", "md5Checksum").
				Context(ctx)
			response, callErr = call.Do()
			return callErr
		}

		call := s.srv.Files.Create(&drive.File{
			Name:    request.Name,
			Parents: []string{request.FolderID},
		}).
			SupportsAllDrives(true).
			Media(fileHandle).
			Fields("id", "name", "modifiedTime", "md5Checksum").
			Context(ctx)
		response, callErr = call.Do()
		return callErr
	})
	if err != nil {
		return nil, err
	}

	logger.Debug("File operation completed successfully", "name", request.Name, "id", response.Id)
	return &types.DriveFile{
		ID:           response.Id,
		Name:         response.Name,
		Path:         request.RemotePath,
		ModifiedTime: parseDriveModifiedTime(response.ModifiedTime),
		MD5Checksum:  response.Md5Checksum,
		IsDirectory:  false,
	}, nil
}
