package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/appstate"
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
	pauseFile string,
) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Failed to create watcher", "error", err)
		return
	}
	defer func() { _ = watcher.Close() }()

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
	var debounceMu sync.Mutex

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if pauseFile != "" && appstate.IsPaused(pauseFile) {
				continue
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

			if sharedState != nil && sharedState.IsActiveDownload(relativePath) {
				continue
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					if err := watcher.Add(event.Name); err != nil {
						logger.Warn("Failed to watch new directory", "path", event.Name, "error", err)
					}
					logger.Debug("Added watch for new directory", "path", event.Name)
				}
			}

			debounceMu.Lock()
			if timer, ok := debounceTimers[relativePath]; ok {
				timer.Stop()
			}

			debounceOps[relativePath] |= event.Op
			relPathCopy := relativePath
			mergedEvent := fsnotify.Event{Name: event.Name, Op: debounceOps[relativePath]}

			debounceTimers[relativePath] = time.AfterFunc(time.Duration(cfg.WatchDebounceDelay)*time.Millisecond, func() {
				debounceMu.Lock()
				delete(debounceTimers, relPathCopy)
				delete(debounceOps, relPathCopy)
				debounceMu.Unlock()
				if sharedState != nil {
					sharedState.RunMutation(func() { handleWatchEvent(mergedEvent, localPath, relPathCopy, driveService, sharedState, cfg, dbStore) })
				} else {
					handleWatchEvent(mergedEvent, localPath, relPathCopy, driveService, sharedState, cfg, dbStore)
				}
			})
			debounceMu.Unlock()

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
			var fileMetadata *types.FileMetadata
			sharedState.ReadRemoteFiles(func(_ types.DriveFileMap, metadata map[string]*types.FileMetadata) {
				fileMetadata = metadata[relativePath]
			})
			if err := dbStore.SaveFileState(relativePath, uploadedFile, fileMetadata); err != nil {
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
