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
	driveService   api.DriveClient
	remoteFiles    types.DriveFileMap
	metadata       map[string]*types.FileMetadata
	cfg            *config.Config
	metaMu         sync.Mutex
	localPath      string
	sharedState    *SharedState
	dryRun         bool
	onTaskComplete func(task types.SyncTask, err error)
}

func (executor *Executor) SetTaskCompleteCallback(callback func(task types.SyncTask, err error)) {
	executor.onTaskComplete = callback
}

func NewExecutor(
	driveService api.DriveClient,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	localPath string,
	sharedState *SharedState,
	dryRun bool,
) *Executor {
	return &Executor{
		driveService: driveService,
		remoteFiles:  remoteFiles,
		metadata:     metadata,
		cfg:          cfg,
		localPath:    localPath,
		sharedState:  sharedState,
		dryRun:       dryRun,
	}
}

func (executor *Executor) ExecuteTasks(tasks []types.SyncTask) error {
	if len(tasks) == 0 {
		logger.Info("All files are up to date.")
		return nil
	}

	logger.Info(fmt.Sprintf("Executing %d sync tasks...", len(tasks)))
	if executor.dryRun {
		for index, task := range tasks {
			logger.Info(fmt.Sprintf("[DRY-RUN %d/%d] %s: %s", index+1, len(tasks), task.Action.String(), task.FilePath))
		}
		return nil
	}

	group, ctx := errgroup.WithContext(context.Background())
	downloadSlots := make(chan struct{}, executor.cfg.MaxConcurrentDownloads)
	uploadSlots := make(chan struct{}, executor.cfg.MaxConcurrentUploads)
	deletionTasks := make([]struct {
		task  types.SyncTask
		index int
	}, 0)

	for index, task := range tasks {
		taskCopy := task
		taskIndex := index + 1

		switch task.Action {
		case types.ActionDownloadNew, types.ActionDownloadUpdate:
			group.Go(func() error {
				if err := acquireTransferSlot(ctx, downloadSlots); err != nil {
					return err
				}
				defer func() { <-downloadSlots }()
				err := executor.downloadWithRetry(ctx, taskCopy, taskIndex, len(tasks))
				executor.reportTaskComplete(taskCopy, err)
				return err
			})

		case types.ActionUploadNew, types.ActionUploadUpdate, types.ActionUploadConflict:
			group.Go(func() error {
				if err := acquireTransferSlot(ctx, uploadSlots); err != nil {
					return err
				}
				defer func() { <-uploadSlots }()
				err := executor.uploadWithRetry(ctx, taskCopy, taskIndex, len(tasks))
				executor.reportTaskComplete(taskCopy, err)
				return err
			})

		case types.ActionDeleteLocal:
			deletionTasks = append(deletionTasks, struct {
				task  types.SyncTask
				index int
			}{taskCopy, taskIndex})

		case types.ActionDeleteRemote:
			deletionTasks = append(deletionTasks, struct {
				task  types.SyncTask
				index int
			}{taskCopy, taskIndex})

		case types.ActionCreateLocalFolder:
			group.Go(func() error {
				err := executor.createLocalFolder(taskCopy, taskIndex, len(tasks))
				executor.reportTaskComplete(taskCopy, err)
				return err
			})
		}
	}

	if err := group.Wait(); err != nil {
		return err
	}
	for _, deletion := range deletionTasks {
		if deletion.task.Action == types.ActionDeleteLocal {
			if err := executor.handleDeleteLocal(deletion.task, deletion.index, len(tasks)); err != nil {
				executor.reportTaskComplete(deletion.task, err)
				return err
			}
		} else {
			if err := executor.handleDeleteRemote(deletion.task, deletion.index, len(tasks)); err != nil {
				executor.reportTaskComplete(deletion.task, err)
				return err
			}
		}
		executor.reportTaskComplete(deletion.task, nil)
	}
	return nil
}

func (executor *Executor) reportTaskComplete(task types.SyncTask, err error) {
	if executor.onTaskComplete != nil {
		executor.onTaskComplete(task, err)
	}
}

func (executor *Executor) createLocalFolder(task types.SyncTask, index, total int) error {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFolderPath, err := localPathWithinRoot(executor.localPath, task.FilePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(localFolderPath, 0755); err != nil {
		return fmt.Errorf("create local folder %s: %w", task.FilePath, err)
	}
	executor.metaMu.Lock()
	executor.metadata[task.FilePath] = &types.FileMetadata{}
	executor.metaMu.Unlock()
	return nil
}

func acquireTransferSlot(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (executor *Executor) downloadWithRetry(ctx context.Context, task types.SyncTask, index, total int) error {
	localFilePath, err := localPathWithinRoot(executor.localPath, task.FilePath)
	if err != nil {
		return err
	}
	executor.metaMu.Lock()
	remoteFile := executor.remoteFiles[task.FilePath]
	executor.metaMu.Unlock()

	if executor.sharedState != nil {
		executor.sharedState.AddActiveDownload(task.FilePath)
		defer executor.sharedState.RemoveActiveDownload(task.FilePath)
	}

	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))

	if remoteFile == nil || remoteFile.ID == "" {
		return fmt.Errorf("cannot download %s: remote ID missing", task.FilePath)
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
			if err := waitForRetry(ctx, 10*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("failed to download %s after 10 attempts: %w", task.FilePath, lastErr)
}

func (executor *Executor) uploadWithRetry(ctx context.Context, task types.SyncTask, index, total int) error {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath, err := localPathWithinRoot(executor.localPath, task.FilePath)
	if err != nil {
		return err
	}
	executor.metaMu.Lock()
	remoteFile := executor.remoteFiles[task.FilePath]
	parentFolderID, ok := resolveRemoteParentFolderID(task.FilePath, executor.remoteFiles, executor.cfg.RemoteFolderID)
	executor.metaMu.Unlock()
	if !ok {
		return fmt.Errorf("cannot upload %s: remote parent folder is unavailable", task.FilePath)
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
			if err := waitForRetry(ctx, 10*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("failed to upload %s after 10 attempts: %w", task.FilePath, lastErr)
}

func (executor *Executor) handleDeleteLocal(task types.SyncTask, index, total int) error {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	localFilePath := filepath.Join(executor.localPath, task.FilePath)

	// Move to trash instead of permanently deleting
	// This is only triggered when the remote file was deleted (not updated)
	if err := utils.MoveToTrash(executor.localPath, localFilePath); err != nil {
		if !os.IsNotExist(err) {
			logger.Error("Failed to move local file to trash", "path", task.FilePath, "error", err)
			return fmt.Errorf("trash local file %s: %w", task.FilePath, err)
		}
	}

	executor.metaMu.Lock()
	delete(executor.metadata, task.FilePath)
	executor.metaMu.Unlock()
	return nil
}

func (executor *Executor) handleDeleteRemote(task types.SyncTask, index, total int) error {
	logger.Info(fmt.Sprintf("[%d/%d] %s: %s", index, total, task.Action.String(), task.FilePath))
	executor.metaMu.Lock()
	remoteFile := executor.remoteFiles[task.FilePath]
	executor.metaMu.Unlock()

	if remoteFile != nil && remoteFile.ID != "" {
		if err := executor.driveService.TrashRemoteFile(context.Background(), remoteFile.ID); err != nil {
			logger.Error("Failed to trash remote file", "path", task.FilePath, "error", err)
			return fmt.Errorf("trash remote file %s: %w", task.FilePath, err)
		}

		executor.metaMu.Lock()
		delete(executor.metadata, task.FilePath)
		delete(executor.remoteFiles, task.FilePath)

		if remoteFile.IsDirectory {
			for key := range executor.remoteFiles {
				if isRemoteDescendant(key, task.FilePath) {
					delete(executor.remoteFiles, key)
					delete(executor.metadata, key)
				}
			}
		}
		executor.metaMu.Unlock()
	} else {
		return fmt.Errorf("cannot trash remote file %s: remote ID missing", task.FilePath)
	}
	return nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func localPathWithinRoot(root, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("unsafe absolute sync path %q", relativePath)
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe sync path %q", relativePath)
	}
	return filepath.Join(root, cleaned), nil
}
