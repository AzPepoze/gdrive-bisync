package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

func WatchLocalFiles(
	localPath string,
	driveService *api.DriveService,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
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
	var timerMu sync.Mutex

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

			timerMu.Lock()
			if t, ok := debounceTimers[relativePath]; ok {
				t.Stop()
			}

			evt := event
			rPath := relativePath

			debounceTimers[relativePath] = time.AfterFunc(time.Duration(cfg.WatchDebounceDelay)*time.Millisecond, func() {
				timerMu.Lock()
				delete(debounceTimers, rPath)
				timerMu.Unlock()

				handleEvent(evt, localPath, rPath, driveService, remoteFiles, metadata, cfg)
			})
			timerMu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error("Watcher error", "error", err)
		}
	}
}

func handleEvent(
	event fsnotify.Event,
	localRoot string,
	relativePath string,
	driveService *api.DriveService,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
) {
	logger.Info(fmt.Sprintf("Processing change: %s %s", event.Op.String(), relativePath))

	localFilePath := filepath.Join(localRoot, relativePath)
	remoteFile := remoteFiles[relativePath]

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		info, err := os.Stat(localFilePath)
		if err != nil {
			return
		}
		if info.Size() == 0 {
			logger.Warn("Skipping 0-byte file", "path", relativePath)
			return
		}

		logger.Info("Uploading", "path", relativePath)
		parentPath := filepath.Dir(relativePath)
		parentFolderID := cfg.RemoteFolderID

		if parentPath != "." {
			if parent, ok := remoteFiles[parentPath]; ok {
				parentFolderID = parent.ID
			} else {
				logger.Error("Could not find remote parent folder", "path", relativePath)
				return
			}
		}

		remoteInfo := struct {
			Name     string
			FolderID string
			FileID   string
		}{
			Name:     filepath.Base(relativePath),
			FolderID: parentFolderID,
		}
		if remoteFile != nil {
			remoteInfo.FileID = remoteFile.ID
		}

		uploadedFile, err := driveService.UploadOrUpdateFile(localFilePath, remoteInfo)
		if err != nil {
			logger.Error("Upload failed", "path", relativePath, "error", err)
			return
		}

		metadata[relativePath] = &types.FileMetadata{RemoteMD5Checksum: uploadedFile.Md5Checksum}

		remoteFiles[relativePath] = &types.DriveFile{
			ID:           uploadedFile.Id,
			Name:         uploadedFile.Name,
			Path:         relativePath,
			ModifiedTime: time.Now(), 
			MD5Checksum:  uploadedFile.Md5Checksum,
			IsDirectory:  false,
		}
		logger.Info("Uploaded", "path", relativePath)

	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		if remoteFile != nil {
			logger.Info("Deleting remote", "path", relativePath)
			if err := driveService.TrashRemoteFile(remoteFile.ID); err != nil {
				logger.Error("Failed to trash remote file", "path", relativePath, "error", err)
			} else {
				delete(metadata, relativePath)
				delete(remoteFiles, relativePath)
				if remoteFile.IsDirectory {
					for k := range remoteFiles {
						if strings.HasPrefix(k, relativePath+string(os.PathSeparator)) {
							delete(remoteFiles, k)
							delete(metadata, k)
						}
					}
				}
				logger.Info("Deleted remote", "path", relativePath)
			}
		}
	}
}