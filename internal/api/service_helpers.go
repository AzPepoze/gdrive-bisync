package api

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

const (
	driveFileFields   = "nextPageToken, files(id, name, mimeType, modifiedTime, md5Checksum)"
	driveChangeFields = "nextPageToken, newStartPageToken, changes(fileId, removed, file(id, name, parents, mimeType, modifiedTime, md5Checksum, trashed))"
)

func (s *DriveService) retry(ctx context.Context, operation string, cfg retryConfig, fn func() error) error {
	var lastErr error
	backoff := time.Second

	for attempt := 1; attempt <= cfg.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		logger.Debug("Retrying Drive API call", "operation", operation, "attempt", attempt, "error", lastErr)

		if attempt == cfg.maxRetries {
			break
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		if backoff < 8*time.Second {
			backoff *= 2
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, cfg.maxRetries, lastErr)
}

func driveFileFieldsSelector() googleapi.Field {
	return googleapi.Field(driveFileFields)
}

func driveChangeFieldsSelector() googleapi.Field {
	return googleapi.Field(driveChangeFields)
}

func applySharedDriveOptionsToFilesList(call *drive.FilesListCall) *drive.FilesListCall {
	return call.SupportsAllDrives(true).IncludeItemsFromAllDrives(true)
}

func applySharedDriveOptionsToChangesList(call *drive.ChangesListCall) *drive.ChangesListCall {
	return call.SupportsAllDrives(true).IncludeItemsFromAllDrives(true)
}

func buildDriveFile(path string, file *drive.File) *types.DriveFile {
	return &types.DriveFile{
		ID:           file.Id,
		Name:         file.Name,
		Path:         path,
		ModifiedTime: parseDriveModifiedTime(file.ModifiedTime),
		MD5Checksum:  file.Md5Checksum,
		IsDirectory:  file.MimeType == "application/vnd.google-apps.folder",
	}
}
