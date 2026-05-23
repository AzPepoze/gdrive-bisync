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
	"gdrive-bisync/internal/utils"
)

type Executor struct {
	driveService api.DriveClient
	remoteFiles  types.DriveFileMap
	metadata     map[string]*types.FileMetadata
	cfg          *config.Config
	metaMu       sync.Mutex
	localPath    string
	sharedState  *SharedState
}

func NewExecutor(
	driveService api.DriveClient,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	localPath string,
	sharedState *SharedState,
) *Executor {
	return &Executor{
		driveService: driveService,
		remoteFiles:  remoteFiles,
		metadata:     metadata,
		cfg:          cfg,
		localPath:    localPath,
		sharedState:  sharedState,
	}
}

func (executor *Executor) ExecuteTasks(tasks []types.SyncTask) error {
	if len(tasks) == 0 {
		logger.Info("All files are up to date.")
		return nil
	}

	logger.Info(fmt.Sprintf("Executing %d sync tasks...", len(tasks)))

	group, ctx := errgroup.WithContext(context.Background())
	group.SetLimit(executor.cfg.MaxConcurrentDownloads)

	for index, task := range tasks {
		taskCopy := task
		taskIndex := index + 1

		switch task.Action {
		case types.ActionDownloadNew, types.ActionDownloadUpdate:
			group.Go(func() error {
				return executor.downloadWithRetry(ctx, taskCopy, taskIndex, len(tasks))
			})

		case types.ActionUploadNew, types.ActionUploadUpdate, types.ActionUploadConflict:
			group.Go(func() error {
				return executor.uploadWithRetry(ctx, taskCopy, taskIndex, len(tasks))
			})

		case types.ActionDeleteLocal:
			executor.handleDeleteLocal(taskCopy, taskIndex, len(tasks))

		case types.ActionDeleteRemote:
			executor.handleDeleteRemote(taskCopy, taskIndex, len(tasks))
		}
	}

	return group.Wait()
}

func (executor *Executor) downloadWithRetry(ctx context.Context, task types.SyncTask, index, total int) error {
	localFilePath := filepath.Join(executor.localPath, task.FilePath)
	remoteFile := executor.remoteFiles[task.FilePath]

	if executor.sharedState != nil {
		executor.sharedState.AddActiveDownload(task.FilePath)
		defer executor.sharedState.RemoveActiveDownload(task.FilePath)
	}

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
		err := executor.driveService.DownloadFile(ctx, api.DownloadFileRequest{
			FileID:          remoteFile.ID,
			DestinationPath: localFilePath,
		})
		if err == nil {
			executor.metaMu.Lock()
			upsertRemoteMetadata(executor.metadata, task.FilePath, remoteFile.MD5Checksum)
			executor.metaMu.Unlock()
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

func (executor *Executor) uploadWithRetry(ctx context.Context, task types.SyncTask, index, total int) error {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath := filepath.Join(executor.localPath, task.FilePath)
	remoteFile := executor.remoteFiles[task.FilePath]

	parentFolderID, ok := resolveRemoteParentFolderID(task.FilePath, executor.remoteFiles, executor.cfg.RemoteFolderID)
	if !ok {
		logger.Error("Could not find remote parent folder", "path", task.FilePath)
		return nil
	}

	request := api.UploadFileRequest{
		LocalPath:  localFilePath,
		RemotePath: task.FilePath,
		Name:       filepath.Base(task.FilePath),
		FolderID:   parentFolderID,
	}
	if remoteFile != nil {
		request.FileID = remoteFile.ID
	}

	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logger.Info("Uploading file to Google Drive", "path", task.FilePath, "attempt", attempt)
		uploadedFile, err := executor.driveService.UploadOrUpdateFile(ctx, request)
		if err == nil {
			executor.metaMu.Lock()
			executor.remoteFiles[task.FilePath] = uploadedFile
			upsertRemoteMetadata(executor.metadata, task.FilePath, uploadedFile.MD5Checksum)
			executor.metaMu.Unlock()
			return nil
		}

		lastErr = err
		logger.Error("Failed to upload file", "path", task.FilePath, "error", err, "attempt", attempt)
		if attempt < 10 {
			time.Sleep(10 * time.Second)
		}
	}
	return fmt.Errorf("failed to upload %s after 10 attempts: %w", task.FilePath, lastErr)
}

func (executor *Executor) handleDeleteLocal(task types.SyncTask, index, total int) {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath := filepath.Join(executor.localPath, task.FilePath)

	// Move to trash instead of permanently deleting
	// This is only triggered when the remote file was deleted (not updated)
	if err := utils.MoveToTrash(executor.localPath, localFilePath); err != nil {
		if !os.IsNotExist(err) {
			logger.Error("Failed to move local file to trash", "path", task.FilePath, "error", err)
			return
		}
	}

	executor.metaMu.Lock()
	delete(executor.metadata, task.FilePath)
	executor.metaMu.Unlock()
}

func (executor *Executor) handleDeleteRemote(task types.SyncTask, index, total int) {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	remoteFile := executor.remoteFiles[task.FilePath]

	if remoteFile != nil && remoteFile.ID != "" {
		if err := executor.driveService.TrashRemoteFile(context.Background(), remoteFile.ID); err != nil {
			logger.Error("Failed to trash remote file", "path", task.FilePath, "error", err)
			return
		}

		executor.metaMu.Lock()
		delete(executor.metadata, task.FilePath)
		delete(executor.remoteFiles, task.FilePath)

		if remoteFile.IsDirectory {
			prefix := task.FilePath + string(os.PathSeparator)
			for key := range executor.remoteFiles {
				if strings.HasPrefix(key, prefix) {
					delete(executor.remoteFiles, key)
					delete(executor.metadata, key)
				}
			}
		}
		executor.metaMu.Unlock()
	} else {
		logger.Warn("Skipping remote deletion, ID missing", "path", task.FilePath)
	}
}
