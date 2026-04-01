package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/scanner"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func Sync(
	driveService *api.DriveService,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	pageToken *string,
	dbStore *store.Store,
) error {
	resolvedLocalPath := utils.ResolvePath(cfg.LocalSyncPath)
	if resolvedLocalPath == "" || cfg.RemoteFolderID == "" {
		logger.Error("Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.")
		return fmt.Errorf("invalid configuration")
	}

	if err := os.MkdirAll(resolvedLocalPath, 0755); err != nil {
		return fmt.Errorf("failed to create local sync path: %w", err)
	}

	logger.Info("Starting sync cycle...")

	logger.Info("Scanning local files...")
	localFiles, err := scanner.GetLocalFilesRecursive(resolvedLocalPath, cfg.IgnoreRegexps, metadata, func(path string) {
		logger.UpdateStatus(fmt.Sprintf("Scanning local: %s", path))
	})
	if err != nil {
		return err
	}
	logger.UpdateStatus("")
	logger.Info("Local scan complete.", "files", len(localFiles))

	if len(remoteFiles) == 0 || *pageToken == "" {
		logger.Info("Performing full remote scan...", "concurrency", cfg.MaxConcurrentScans, "retries", cfg.MaxRetries)

		token, err := driveService.GetStartPageToken()
		if err != nil {
			logger.Error("Failed to get start page token", "error", err)
		} else {
			*pageToken = token
		}

		currentRemoteFiles, err := driveService.ListFilesRecursive(
			cfg.RemoteFolderID,
			cfg.IgnoreRegexps,
			func(path string) {
				logger.UpdateStatus(fmt.Sprintf("Scanning remote: %s", path))
			},
			cfg.MaxConcurrentScans,
			cfg.MaxRetries,
		)
		if err != nil {
			return err
		}
		logger.UpdateStatus("")
		logger.Info("Remote scan complete.", "files", len(currentRemoteFiles))

		for key := range remoteFiles {
			delete(remoteFiles, key)
		}
		for key, value := range currentRemoteFiles {
			remoteFiles[key] = value
		}

		if dbStore != nil {
			if err := dbStore.ReplaceAllRemoteFiles(remoteFiles); err != nil {
				logger.Error("Failed to persist remote files after full scan", "error", err)
			}
		}

	} else {
		logger.Info("Checking for remote changes...", "token", *pageToken)
		changes, newToken, err := driveService.FetchChanges(*pageToken)
		if err != nil {
			logger.Error("Failed to fetch changes, falling back to full scan", "error", err)
			*pageToken = ""
			return err
		}

		relevantChanges := filterRelevantChanges(changes, remoteFiles)
		if len(relevantChanges) > 0 {
			logger.Info(fmt.Sprintf("Processing %d relevant remote changes (of %d total)...", len(relevantChanges), len(changes)))
			changedByApply, deletedByApply, deletedMetadataByApply := applyChanges(relevantChanges, remoteFiles, metadata, cfg.RemoteFolderID, cfg.IgnoreRegexps)
			if dbStore != nil {
				if err := dbStore.SaveRemoteFiles(changedByApply, deletedByApply); err != nil {
					logger.Error("Failed to persist incremental remote changes", "error", err)
				}
				if err := dbStore.SaveMetadata(metadata, deletedMetadataByApply); err != nil {
					logger.Error("Failed to persist incremental metadata changes", "error", err)
				}
			}
		} else {
			logger.Info("No remote changes found.")
		}
		*pageToken = newToken
	}

	changedMetadata := make(map[string]*types.FileMetadata)
	deletedMetadataPaths := make([]string, 0)

	for path, local := range localFiles {
		if local.IsDirectory {
			if _, remoteExists := remoteFiles[path]; remoteExists {
				if _, metaExists := metadata[path]; !metaExists {
					metadata[path] = &types.FileMetadata{}
				}
			}
		}
	}

	localFolders := make([]*types.LocalFile, 0)
	for _, file := range localFiles {
		if file.IsDirectory {
			localFolders = append(localFolders, file)
		}
	}
	sort.Slice(localFolders, func(i, j int) bool {
		return strings.Count(localFolders[i].Path, string(os.PathSeparator)) < strings.Count(localFolders[j].Path, string(os.PathSeparator))
	})

	for _, folder := range localFolders {
		if _, exists := remoteFiles[folder.Path]; !exists {
			if _, inMetadata := metadata[folder.Path]; inMetadata {
				childPrefix := folder.Path + string(os.PathSeparator)
				childrenStillInRemote := false
				for remotePath := range remoteFiles {
					if strings.HasPrefix(remotePath, childPrefix) {
						childrenStillInRemote = true
						break
					}
				}
				if childrenStillInRemote {
					logger.Warn("Folder missing from remote map but children still present, skipping deletion", "path", folder.Path)
					continue
				}

				logger.Info("Folder deleted remotely. Moving to trash.", "path", folder.Path)
				fullPath := filepath.Join(resolvedLocalPath, folder.Path)
				if err := utils.MoveToTrash(resolvedLocalPath, fullPath); err != nil {
					logger.Error("Failed to move local folder to trash", "path", folder.Path, "error", err)
				} else {
					delete(metadata, folder.Path)
					deletedMetadataPaths = append(deletedMetadataPaths, folder.Path)
					for key := range metadata {
						if strings.HasPrefix(key, childPrefix) {
							delete(metadata, key)
							deletedMetadataPaths = append(deletedMetadataPaths, key)
						}
					}
				}
				continue
			}

			parentPath := filepath.Dir(folder.Path)
			parentFolderID := cfg.RemoteFolderID
			if parentPath != "." {
				if parent, ok := remoteFiles[parentPath]; ok {
					parentFolderID = parent.ID
				} else {
					logger.Warn("Skipping folder creation, parent not found", "folder", folder.Path)
					continue
				}
			}

			logger.Info("Creating remote folder", "path", folder.Path)
			id, err := driveService.CreateFolder(parentFolderID, filepath.Base(folder.Path))
			if err != nil {
				logger.Error("Failed to create remote folder", "path", folder.Path, "error", err)
				continue
			}

			newDriveFile := &types.DriveFile{
				ID:           id,
				Name:         filepath.Base(folder.Path),
				Path:         folder.Path,
				ModifiedTime: time.Now(),
				IsDirectory:  true,
			}
			remoteFiles[folder.Path] = newDriveFile
			if dbStore != nil {
				if err := dbStore.SaveRemoteFiles(types.DriveFileMap{folder.Path: newDriveFile}, nil); err != nil {
					logger.Error("Failed to persist new remote folder", "path", folder.Path, "error", err)
				}
			}
			metadata[folder.Path] = &types.FileMetadata{}
			changedMetadata[folder.Path] = metadata[folder.Path]
		}
	}

	allPaths := make(map[string]struct{})
	for key := range localFiles {
		allPaths[key] = struct{}{}
	}
	for key := range remoteFiles {
		allPaths[key] = struct{}{}
	}

	var tasks []types.SyncTask

	for pathStr := range allPaths {
		local := localFiles[pathStr]
		remote := remoteFiles[pathStr]

		ignored := false
		for _, re := range cfg.IgnoreRegexps {
			if re.MatchString(pathStr) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		isDir := (local != nil && local.IsDirectory) || (remote != nil && remote.IsDirectory)

		if isDir {
			if local == nil && remote != nil {
				if _, inMetadata := metadata[pathStr]; inMetadata {
					tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: pathStr})
				}
			}
			continue
		}

		action := DetermineSyncAction(pathStr, local, remote, metadata)
		if action != types.ActionSkipNoChange && action != types.ActionSkipIdentical {
			tasks = append(tasks, types.SyncTask{Action: action, FilePath: pathStr})
		}
	}

	executor := NewExecutor(driveService, remoteFiles, metadata, cfg, resolvedLocalPath)
	if err := executor.ExecuteTasks(tasks); err != nil {
		logger.Error("Sync tasks execution failed", "error", err)
	}

	if dbStore != nil {
		if err := dbStore.ReplaceAllRemoteFiles(remoteFiles); err != nil {
			logger.Error("Failed to save remote files", "error", err)
		}
		if err := dbStore.SaveMetadata(metadata, deletedMetadataPaths); err != nil {
			logger.Error("Failed to save metadata", "error", err)
		}
		if err := dbStore.SavePageToken(*pageToken); err != nil {
			logger.Error("Failed to save page token", "error", err)
		}
	}

	logger.Info("Sync cycle finished.")
	logger.Info("--------------------------------------------------")
	return nil
}

func filterRelevantChanges(changes []*drive.Change, remoteFiles types.DriveFileMap) []*drive.Change {
	knownIDs := make(map[string]struct{}, len(remoteFiles))
	for _, driveFile := range remoteFiles {
		knownIDs[driveFile.ID] = struct{}{}
	}

	relevant := make([]*drive.Change, 0, len(changes))
	for _, change := range changes {
		if _, known := knownIDs[change.FileId]; known {
			relevant = append(relevant, change)
			continue
		}
		if change.File != nil && len(change.File.Parents) > 0 {
			for _, parentID := range change.File.Parents {
				if _, known := knownIDs[parentID]; known {
					relevant = append(relevant, change)
					break
				}
			}
		}
	}
	return relevant
}

func applyChanges(changes []*drive.Change, remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata, rootFolderID string, ignoreRegexps []*regexp.Regexp) (types.DriveFileMap, []string, []string) {
	changedFiles := make(types.DriveFileMap)
	deletedPaths := make([]string, 0)
	deletedMetadataPaths := make([]string, 0)

	idToPath := make(map[string]string)
	for path, driveFile := range remoteFiles {
		idToPath[driveFile.ID] = path
	}

	for _, change := range changes {
		if change.Removed || (change.File != nil && change.File.Trashed) {
			path, exists := idToPath[change.FileId]
			if exists {
				logger.Debug("Applying remote deletion", "path", path)
				delete(remoteFiles, path)
				delete(metadata, path)
				delete(idToPath, change.FileId)
				deletedPaths = append(deletedPaths, path)
				deletedMetadataPaths = append(deletedMetadataPaths, path)

				childPrefix := path + string(os.PathSeparator)
				for key := range remoteFiles {
					if strings.HasPrefix(key, childPrefix) {
						delete(remoteFiles, key)
						delete(metadata, key)
						deletedPaths = append(deletedPaths, key)
						deletedMetadataPaths = append(deletedMetadataPaths, key)
					}
				}
			}
			continue
		}

		if change.File != nil {
			file := change.File

			parentID := ""
			if len(file.Parents) > 0 {
				parentID = file.Parents[0]
			}

			parentPath := ""
			if parentID == rootFolderID {
				parentPath = ""
			} else {
				pPath, ok := idToPath[parentID]
				if !ok {
					logger.Debug("Parent not found in scope, skipping change to avoid false deletion", "fileId", change.FileId, "parentId", parentID)
					continue
				}
				parentPath = pPath
			}

			newPath := filepath.Join(parentPath, file.Name)

			ignored := false
			for _, re := range ignoreRegexps {
				if re.MatchString(newPath) {
					ignored = true
					break
				}
			}
			if ignored {
				if oldPath, exists := idToPath[change.FileId]; exists {
					delete(remoteFiles, oldPath)
					delete(metadata, oldPath)
					delete(idToPath, change.FileId)
					deletedPaths = append(deletedPaths, oldPath)
					deletedMetadataPaths = append(deletedMetadataPaths, oldPath)
				}
				continue
			}

			oldPath, exists := idToPath[change.FileId]
			isDirectory := file.MimeType == "application/vnd.google-apps.folder"
			modTime, _ := time.Parse(time.RFC3339, file.ModifiedTime)

			if exists && oldPath != newPath {
				logger.Debug("Applying remote move", "old", oldPath, "new", newPath)
				delete(remoteFiles, oldPath)
				deletedPaths = append(deletedPaths, oldPath)
				if oldMeta, metaExists := metadata[oldPath]; metaExists {
					metadata[newPath] = oldMeta
					delete(metadata, oldPath)
					deletedMetadataPaths = append(deletedMetadataPaths, oldPath)
				}

				if isDirectory {
					oldPrefix := oldPath + string(os.PathSeparator)
					newPrefix := newPath + string(os.PathSeparator)

					type childMove struct {
						oldKey string
						newKey string
						value  *types.DriveFile
					}
					var moves []childMove

					for key, value := range remoteFiles {
						if strings.HasPrefix(key, oldPrefix) {
							suffix := strings.TrimPrefix(key, oldPrefix)
							newKey := newPrefix + suffix
							moves = append(moves, childMove{key, newKey, value})
						}
					}

					for _, move := range moves {
						delete(remoteFiles, move.oldKey)
						deletedPaths = append(deletedPaths, move.oldKey)
						if oldMeta, metaExists := metadata[move.oldKey]; metaExists {
							metadata[move.newKey] = oldMeta
							delete(metadata, move.oldKey)
							deletedMetadataPaths = append(deletedMetadataPaths, move.oldKey)
						}
						move.value.Path = move.newKey
						remoteFiles[move.newKey] = move.value
						changedFiles[move.newKey] = move.value
						idToPath[move.value.ID] = move.newKey
					}
				}
			}

			newDriveFile := &types.DriveFile{
				ID:           file.Id,
				Name:         file.Name,
				Path:         newPath,
				ModifiedTime: modTime,
				MD5Checksum:  file.Md5Checksum,
				IsDirectory:  isDirectory,
			}
			remoteFiles[newPath] = newDriveFile
			changedFiles[newPath] = newDriveFile
			idToPath[file.Id] = newPath

			if file.MimeType != "application/vnd.google-apps.folder" {
				existing := metadata[newPath]
				if existing == nil {
					existing = &types.FileMetadata{}
					metadata[newPath] = existing
				}
				existing.RemoteMD5Checksum = file.Md5Checksum
			}
		}
	}

	return changedFiles, deletedPaths, deletedMetadataPaths
}
