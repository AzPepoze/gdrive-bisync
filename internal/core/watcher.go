package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/types"
)

func WatchLocalFiles(
	localPath string,
	driveService api.DriveClient,
	sharedState *SharedState,
	cfg *config.Config,
	dbStore *store.Store,
) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Failed to create watcher", "error", err)
		return
	}
	defer watcher.Close()

	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(localPath, path)
		for _, re := range cfg.IgnoreRegexps {
			if re.MatchString(relPath) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		logger.Error("Failed to add watch paths", "error", err)
	}

	logger.Info(fmt.Sprintf("Watching for local changes in: %s", localPath))

	debounceTimers := make(map[string]*time.Timer)
	debounceOps := make(map[string]fsnotify.Op)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			relativePath, err := filepath.Rel(localPath, event.Name)
			if err != nil {
				continue
			}

			ignored := false
			for _, re := range cfg.IgnoreRegexps {
				if re.MatchString(relativePath) {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					watcher.Add(event.Name)
					logger.Debug("Added watch for new directory", "path", event.Name)
				}
			}

			if timer, ok := debounceTimers[relativePath]; ok {
				timer.Stop()
			}

			debounceOps[relativePath] |= event.Op
			relPathCopy := relativePath
			mergedEvent := fsnotify.Event{Name: event.Name, Op: debounceOps[relativePath]}

			debounceTimers[relativePath] = time.AfterFunc(time.Duration(cfg.WatchDebounceDelay)*time.Millisecond, func() {
				delete(debounceTimers, relPathCopy)
				delete(debounceOps, relPathCopy)
				handleWatchEvent(mergedEvent, localPath, relPathCopy, driveService, sharedState, cfg, dbStore)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error("Watcher error", "error", err)
		}
	}
}

func handleWatchEvent(
	event fsnotify.Event,
	localRoot string,
	relativePath string,
	driveService api.DriveClient,
	sharedState *SharedState,
	cfg *config.Config,
	dbStore *store.Store,
) {
	logger.Info(fmt.Sprintf("Processing change: %s %s", event.Op.String(), relativePath))

	localFilePath := filepath.Join(localRoot, relativePath)

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		info, err := os.Stat(localFilePath)
		if err != nil {
			return
		}
		if info.IsDir() {
			logger.Info("Deferring directory reconciliation to sync cycle", "path", relativePath, "op", event.Op.String())
			return
		}
		if info.Size() == 0 {
			logger.Warn("Skipping 0-byte file", "path", relativePath)
			return
		}

		var remoteFile *types.DriveFile
		sharedState.ReadRemoteFiles(func(remoteFiles types.DriveFileMap, _ map[string]*types.FileMetadata) {
			remoteFile = remoteFiles[relativePath]
		})
		if remoteFile != nil && remoteFile.IsDirectory {
			logger.Info("Deferring file/directory replacement to sync cycle", "path", relativePath)
			return
		}

		logger.Info("Uploading", "path", relativePath)
		parentFolderID, ok := resolveParentFolderIDFromSharedState(relativePath, sharedState, cfg.RemoteFolderID)
		if !ok {
			logger.Info("Deferring upload until sync cycle because remote parent folder is unavailable", "path", relativePath, "parent", filepath.Dir(relativePath))
			return
		}

		request := api.UploadFileRequest{
			LocalPath:  localFilePath,
			RemotePath: relativePath,
			Name:       filepath.Base(relativePath),
			FolderID:   parentFolderID,
		}
		if remoteFile != nil {
			request.FileID = remoteFile.ID
		}

		uploadedFile, err := driveService.UploadOrUpdateFile(context.Background(), request)
		if err != nil {
			logger.Error("Upload failed", "path", relativePath, "error", err)
			return
		}

		sharedState.WriteRemoteFiles(func(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata) string {
			remoteFiles[relativePath] = uploadedFile
			upsertRemoteMetadata(metadata, relativePath, uploadedFile.MD5Checksum)
			return ""
		})

		if dbStore != nil {
			changedRemote := types.DriveFileMap{relativePath: uploadedFile}
			if err := dbStore.SaveRemoteFiles(changedRemote, nil); err != nil {
				logger.Error("Failed to persist remote file to store", "path", relativePath, "error", err)
			}
		}

		logger.Info("Uploaded", "path", relativePath)

	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		// Deletions and renames are intentionally handled by the sync cycle.
		// This avoids destructive races where a local rename is interpreted as an immediate remote delete.
		logger.Info("Deferring remove/rename reconciliation to sync cycle", "path", relativePath, "op", event.Op.String())
	}
}
