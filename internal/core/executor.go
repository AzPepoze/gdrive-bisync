package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

type Executor struct {
	driveService *api.DriveService
	remoteFiles  types.DriveFileMap
	metadata     map[string]*types.FileMetadata
	cfg          *config.Config
	metaMu       sync.Mutex
	localPath    string
}

func NewExecutor(
	driveService *api.DriveService,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	localPath string,
) *Executor {
	return &Executor{
		driveService: driveService,
		remoteFiles:  remoteFiles,
		metadata:     metadata,
		cfg:          cfg,
		localPath:    localPath,
	}
}

func (e *Executor) ExecuteTasks(tasks []types.SyncTask) error {
	if len(tasks) == 0 {
		logger.Info("All files are up to date.")
		return nil
	}

	logger.Info(fmt.Sprintf("Executing %d sync tasks...", len(tasks)))

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(e.cfg.MaxConcurrentDownloads)

	for i, task := range tasks {
		task := task // capture range variable
		index := i + 1

		switch task.Action {
		case types.ActionDownloadNew, types.ActionDownloadUpdate:
			g.Go(func() error {
				return e.downloadWithRetry(ctx, task, index, len(tasks))
			})

		case types.ActionUploadNew, types.ActionUploadUpdate, types.ActionUploadConflict:
			// Sequential uploads for now to avoid complexity, but could be parallelized later
			e.handleUpload(task, index, len(tasks))

		case types.ActionDeleteLocal:
			e.handleDeleteLocal(task, index, len(tasks))

		case types.ActionDeleteRemote:
			e.handleDeleteRemote(task, index, len(tasks))
		}
	}

	if err := g.Wait(); err != nil {
		logger.Error("Some parallel tasks failed", "error", err)
		return err
	}

	return nil
}

func (e *Executor) downloadWithRetry(ctx context.Context, task types.SyncTask, index, total int) error {
	localFilePath := filepath.Join(e.localPath, task.FilePath)
	remoteFile := e.remoteFiles[task.FilePath]

	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))

	if remoteFile == nil || remoteFile.ID == "" {
		logger.Warn("Skipping download, remote ID missing", "path", task.FilePath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(localFilePath), 0755); err != nil {
		logger.Error("Failed to create dir", "path", filepath.Dir(localFilePath), "error", err)
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logger.Info("Downloading file from Google Drive", "path", task.FilePath, "attempt", attempt)
		err := e.driveService.DownloadFile(remoteFile.ID, localFilePath)
		if err == nil {
			e.metaMu.Lock()
			e.metadata[task.FilePath] = &types.FileMetadata{RemoteMD5Checksum: remoteFile.MD5Checksum}
			e.metaMu.Unlock()
			return nil
		}

		lastErr = err
		logger.Error("Failed to download file", "path", task.FilePath, "error", err, "attempt", attempt)
		if attempt < 10 {
			time.Sleep(10 * time.Second)
		}
	}
	return fmt.Errorf("failed to download %s after 10 attempts: %w", task.FilePath, lastErr)
}

func (e *Executor) handleUpload(task types.SyncTask, index, total int) {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath := filepath.Join(e.localPath, task.FilePath)
	remoteFile := e.remoteFiles[task.FilePath]

	parentPath := filepath.Dir(task.FilePath)
	parentFolderID := e.cfg.RemoteFolderID
	if parentPath != "." {
		if parent, ok := e.remoteFiles[parentPath]; ok {
			parentFolderID = parent.ID
		} else {
			logger.Error("Could not find remote parent folder", "path", task.FilePath)
			return
		}
	}

	remoteInfo := struct {
		Name     string
		FolderID string
		FileID   string
	}{
		Name:     filepath.Base(task.FilePath),
		FolderID: parentFolderID,
	}
	if remoteFile != nil {
		remoteInfo.FileID = remoteFile.ID
	}

	logger.Info("Uploading file to Google Drive", "path", task.FilePath)
	uploadedFile, err := e.driveService.UploadOrUpdateFile(localFilePath, remoteInfo)
	if err != nil {
		logger.Error("Failed to upload/update file", "path", task.FilePath, "error", err)
		return
	}

	e.metaMu.Lock()
	e.metadata[task.FilePath] = &types.FileMetadata{RemoteMD5Checksum: uploadedFile.Md5Checksum}
	e.metaMu.Unlock()
}

func (e *Executor) handleDeleteLocal(task types.SyncTask, index, total int) {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath := filepath.Join(e.localPath, task.FilePath)

	if err := os.Remove(localFilePath); err != nil {
		if !os.IsNotExist(err) {
			logger.Error("Failed to delete local file", "path", task.FilePath, "error", err)
			return
		}
	}

	e.metaMu.Lock()
	delete(e.metadata, task.FilePath)
	e.metaMu.Unlock()
}

func (e *Executor) handleDeleteRemote(task types.SyncTask, index, total int) {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	remoteFile := e.remoteFiles[task.FilePath]

	if remoteFile != nil && remoteFile.ID != "" {
		if err := e.driveService.TrashRemoteFile(remoteFile.ID); err != nil {
			logger.Error("Failed to trash remote file", "path", task.FilePath, "error", err)
			return
		}

		e.metaMu.Lock()
		delete(e.metadata, task.FilePath)
		delete(e.remoteFiles, task.FilePath)

		if remoteFile.IsDirectory {
			prefix := task.FilePath + string(os.PathSeparator)
			for k := range e.remoteFiles {
				if strings.HasPrefix(k, prefix) {
					delete(e.remoteFiles, k)
					delete(e.metadata, k)
				}
			}
		}
		e.metaMu.Unlock()
	} else {
		logger.Warn("Skipping remote deletion, ID missing", "path", task.FilePath)
	}
}
